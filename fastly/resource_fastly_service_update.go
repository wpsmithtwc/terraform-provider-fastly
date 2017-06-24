package fastly

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/schema"
	gofastly "github.com/sethvargo/go-fastly"
)

func resourceServiceV1Update(d *schema.ResourceData, meta interface{}) error {
	if err := validateVCLs(d); err != nil {
		return err
	}

	conn := meta.(*FastlyClient).conn

	// Update Name. No new verions is required for this
	if d.HasChange("name") {
		_, err := conn.UpdateService(&gofastly.UpdateServiceInput{
			ID:   d.Id(),
			Name: d.Get("name").(string),
		})
		if err != nil {
			return err
		}
	}

	// Once activated, Versions are locked and become immutable. This is true for
	// versions that are no longer active. For Domains, Backends, DefaultHost and
	// DefaultTTL, a new Version must be created first, and updates posted to that
	// Version. Loop these attributes and determine if we need to create a new version first
	var needsChange bool
	for _, v := range []string{
		"domain",
		"backend",
		"default_host",
		"default_ttl",
		"header",
		"gzip",
		"healthcheck",
		"s3logging",
		"papertrail",
		"response_object",
		"condition",
		"request_setting",
		"cache_setting",
		"vcl",
	} {
		if d.HasChange(v) {
			needsChange = true
		}
	}

	if needsChange {
		latestVersion := d.Get("active_version").(int)
		if latestVersion == 0 {
			// If the service was just created, there is an empty Version 1 available
			// that is unlocked and can be updated
			latestVersion = 1
		} else {
			// Clone the latest version, giving us an unlocked version we can modify
			log.Printf("[DEBUG] Creating clone of version (%d) for updates", latestVersion)
			newVersion, err := conn.CloneVersion(&gofastly.CloneVersionInput{
				Service: d.Id(),
				Version: latestVersion,
			})
			if err != nil {
				return err
			}

			// The new version number is named "Number", but it's actually a string
			latestVersion = newVersion.Number

			// New versions are not immediately found in the API, or are not
			// immediately mutable, so we need to sleep a few and let Fastly ready
			// itself. Typically, 7 seconds is enough
			log.Print("[DEBUG] Sleeping 7 seconds to allow Fastly Version to be available")
			time.Sleep(7 * time.Second)
		}

		// update general settings
		if d.HasChange("default_host") || d.HasChange("default_ttl") {
			opts := gofastly.UpdateSettingsInput{
				Service: d.Id(),
				Version: latestVersion,
				// default_ttl has the same default value of 3600 that is provided by
				// the Fastly API, so it's safe to include here
				DefaultTTL: uint(d.Get("default_ttl").(int)),
			}

			if attr, ok := d.GetOk("default_host"); ok {
				opts.DefaultHost = attr.(string)
			}

			log.Printf("[DEBUG] Update Settings opts: %#v", opts)
			_, err := conn.UpdateSettings(&opts)
			if err != nil {
				return err
			}
		}

		// Conditions need to be updated first, as they can be referenced by other
		// configuraiton objects (Backends, Request Headers, etc)

		// Find difference in Conditions
		if d.HasChange("condition") {
			// Note: we don't utilize the PUT endpoint to update these objects, we simply
			// destroy any that have changed, and create new ones with the updated
			// values. This is how Terraform works with nested sub resources, we only
			// get the full diff not a partial set item diff. Because this is done
			// on a new version of the Fastly Service configuration, this is considered safe

			oc, nc := d.GetChange("condition")
			if oc == nil {
				oc = new(schema.Set)
			}
			if nc == nil {
				nc = new(schema.Set)
			}

			ocs := oc.(*schema.Set)
			ncs := nc.(*schema.Set)
			removeConditions := ocs.Difference(ncs).List()
			addConditions := ncs.Difference(ocs).List()

			// DELETE old Conditions
			for _, cRaw := range removeConditions {
				cf := cRaw.(map[string]interface{})
				opts := gofastly.DeleteConditionInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    cf["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Conditions Removal opts: %#v", opts)
				err := conn.DeleteCondition(&opts)
				if err != nil {
					return err
				}
			}

			// POST new Conditions
			for _, cRaw := range addConditions {
				cf := cRaw.(map[string]interface{})
				opts := gofastly.CreateConditionInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    cf["name"].(string),
					Type:    cf["type"].(string),
					// need to trim leading/tailing spaces, incase the config has HEREDOC
					// formatting and contains a trailing new line
					Statement: strings.TrimSpace(cf["statement"].(string)),
					Priority:  cf["priority"].(int),
				}

				log.Printf("[DEBUG] Create Conditions Opts: %#v", opts)
				_, err := conn.CreateCondition(&opts)
				if err != nil {
					return err
				}
			}
		}

		// Find differences in domains
		if d.HasChange("domain") {
			od, nd := d.GetChange("domain")
			if od == nil {
				od = new(schema.Set)
			}
			if nd == nil {
				nd = new(schema.Set)
			}

			ods := od.(*schema.Set)
			nds := nd.(*schema.Set)

			remove := ods.Difference(nds).List()
			add := nds.Difference(ods).List()

			// Delete removed domains
			for _, dRaw := range remove {
				df := dRaw.(map[string]interface{})
				opts := gofastly.DeleteDomainInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    df["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Domain removal opts: %#v", opts)
				err := conn.DeleteDomain(&opts)
				if err != nil {
					return err
				}
			}

			// POST new Domains
			for _, dRaw := range add {
				df := dRaw.(map[string]interface{})
				opts := gofastly.CreateDomainInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    df["name"].(string),
				}

				if v, ok := df["comment"]; ok {
					opts.Comment = v.(string)
				}

				log.Printf("[DEBUG] Fastly Domain Addition opts: %#v", opts)
				_, err := conn.CreateDomain(&opts)
				if err != nil {
					return err
				}
			}
		}

		// Healthchecks need to be updated BEFORE backends
		if d.HasChange("healthcheck") {
			oh, nh := d.GetChange("healthcheck")
			if oh == nil {
				oh = new(schema.Set)
			}
			if nh == nil {
				nh = new(schema.Set)
			}

			ohs := oh.(*schema.Set)
			nhs := nh.(*schema.Set)
			removeHealthCheck := ohs.Difference(nhs).List()
			addHealthCheck := nhs.Difference(ohs).List()

			// DELETE old healthcheck configurations
			for _, hRaw := range removeHealthCheck {
				hf := hRaw.(map[string]interface{})
				opts := gofastly.DeleteHealthCheckInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    hf["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Healthcheck removal opts: %#v", opts)
				err := conn.DeleteHealthCheck(&opts)
				if err != nil {
					return err
				}
			}

			// POST new/updated Healthcheck
			for _, hRaw := range addHealthCheck {
				hf := hRaw.(map[string]interface{})

				opts := gofastly.CreateHealthCheckInput{
					Service:          d.Id(),
					Version:          latestVersion,
					Name:             hf["name"].(string),
					Host:             hf["host"].(string),
					Path:             hf["path"].(string),
					CheckInterval:    uint(hf["check_interval"].(int)),
					ExpectedResponse: uint(hf["expected_response"].(int)),
					HTTPVersion:      hf["http_version"].(string),
					Initial:          uint(hf["initial"].(int)),
					Method:           hf["method"].(string),
					Threshold:        uint(hf["threshold"].(int)),
					Timeout:          uint(hf["timeout"].(int)),
					Window:           uint(hf["window"].(int)),
				}

				log.Printf("[DEBUG] Create Healthcheck Opts: %#v", opts)
				_, err := conn.CreateHealthCheck(&opts)
				if err != nil {
					return err
				}
			}
		}

		// find difference in backends
		if d.HasChange("backend") {
			ob, nb := d.GetChange("backend")
			if ob == nil {
				ob = new(schema.Set)
			}
			if nb == nil {
				nb = new(schema.Set)
			}

			obs := ob.(*schema.Set)
			nbs := nb.(*schema.Set)
			removeBackends := obs.Difference(nbs).List()
			addBackends := nbs.Difference(obs).List()

			// DELETE old Backends
			for _, bRaw := range removeBackends {
				bf := bRaw.(map[string]interface{})
				opts := gofastly.DeleteBackendInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    bf["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Backend removal opts: %#v", opts)
				err := conn.DeleteBackend(&opts)
				if err != nil {
					return err
				}
			}

			// Find and post new Backends
			for _, dRaw := range addBackends {
				df := dRaw.(map[string]interface{})
				opts := gofastly.CreateBackendInput{
					Service:             d.Id(),
					Version:             latestVersion,
					Name:                df["name"].(string),
					Address:             df["address"].(string),
					AutoLoadbalance:     gofastly.CBool(df["auto_loadbalance"].(bool)),
					SSLCheckCert:        gofastly.CBool(df["ssl_check_cert"].(bool)),
					SSLHostname:         df["ssl_hostname"].(string),
					SSLCertHostname:     df["ssl_cert_hostname"].(string),
					SSLSNIHostname:      df["ssl_sni_hostname"].(string),
					Shield:              df["shield"].(string),
					Port:                uint(df["port"].(int)),
					BetweenBytesTimeout: uint(df["between_bytes_timeout"].(int)),
					ConnectTimeout:      uint(df["connect_timeout"].(int)),
					ErrorThreshold:      uint(df["error_threshold"].(int)),
					FirstByteTimeout:    uint(df["first_byte_timeout"].(int)),
					MaxConn:             uint(df["max_conn"].(int)),
					Weight:              uint(df["weight"].(int)),
					RequestCondition:    df["request_condition"].(string),
					HealthCheck:         df["healthcheck"].(string),
				}

				log.Printf("[DEBUG] Create Backend Opts: %#v", opts)
				_, err := conn.CreateBackend(&opts)
				if err != nil {
					return err
				}
			}
		}

		if d.HasChange("header") {
			oh, nh := d.GetChange("header")
			if oh == nil {
				oh = new(schema.Set)
			}
			if nh == nil {
				nh = new(schema.Set)
			}

			ohs := oh.(*schema.Set)
			nhs := nh.(*schema.Set)

			remove := ohs.Difference(nhs).List()
			add := nhs.Difference(ohs).List()

			// Delete removed headers
			for _, dRaw := range remove {
				df := dRaw.(map[string]interface{})
				opts := gofastly.DeleteHeaderInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    df["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Header removal opts: %#v", opts)
				err := conn.DeleteHeader(&opts)
				if err != nil {
					return err
				}
			}

			// POST new Headers
			for _, dRaw := range add {
				opts, err := buildHeader(dRaw.(map[string]interface{}))
				if err != nil {
					log.Printf("[DEBUG] Error building Header: %s", err)
					return err
				}
				opts.Service = d.Id()
				opts.Version = latestVersion

				log.Printf("[DEBUG] Fastly Header Addition opts: %#v", opts)
				_, err = conn.CreateHeader(opts)
				if err != nil {
					return err
				}
			}
		}

		// Find differences in Gzips
		if d.HasChange("gzip") {
			og, ng := d.GetChange("gzip")
			if og == nil {
				og = new(schema.Set)
			}
			if ng == nil {
				ng = new(schema.Set)
			}

			ogs := og.(*schema.Set)
			ngs := ng.(*schema.Set)

			remove := ogs.Difference(ngs).List()
			add := ngs.Difference(ogs).List()

			// Delete removed gzip rules
			for _, dRaw := range remove {
				df := dRaw.(map[string]interface{})
				opts := gofastly.DeleteGzipInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    df["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Gzip removal opts: %#v", opts)
				err := conn.DeleteGzip(&opts)
				if err != nil {
					return err
				}
			}

			// POST new Gzips
			for _, dRaw := range add {
				df := dRaw.(map[string]interface{})
				opts := gofastly.CreateGzipInput{
					Service:        d.Id(),
					Version:        latestVersion,
					Name:           df["name"].(string),
					CacheCondition: df["cache_condition"].(string),
				}

				if v, ok := df["content_types"]; ok {
					if len(v.(*schema.Set).List()) > 0 {
						var cl []string
						for _, c := range v.(*schema.Set).List() {
							cl = append(cl, c.(string))
						}
						opts.ContentTypes = strings.Join(cl, " ")
					}
				}

				if v, ok := df["extensions"]; ok {
					if len(v.(*schema.Set).List()) > 0 {
						var el []string
						for _, e := range v.(*schema.Set).List() {
							el = append(el, e.(string))
						}
						opts.Extensions = strings.Join(el, " ")
					}
				}

				log.Printf("[DEBUG] Fastly Gzip Addition opts: %#v", opts)
				_, err := conn.CreateGzip(&opts)
				if err != nil {
					return err
				}
			}
		}

		// find difference in s3logging
		if d.HasChange("s3logging") {
			os, ns := d.GetChange("s3logging")
			if os == nil {
				os = new(schema.Set)
			}
			if ns == nil {
				ns = new(schema.Set)
			}

			oss := os.(*schema.Set)
			nss := ns.(*schema.Set)
			removeS3Logging := oss.Difference(nss).List()
			addS3Logging := nss.Difference(oss).List()

			// DELETE old S3 Log configurations
			for _, sRaw := range removeS3Logging {
				sf := sRaw.(map[string]interface{})
				opts := gofastly.DeleteS3Input{
					Service: d.Id(),
					Version: latestVersion,
					Name:    sf["name"].(string),
				}

				log.Printf("[DEBUG] Fastly S3 Logging removal opts: %#v", opts)
				err := conn.DeleteS3(&opts)
				if err != nil {
					return err
				}
			}

			// POST new/updated S3 Logging
			for _, sRaw := range addS3Logging {
				sf := sRaw.(map[string]interface{})

				// Fastly API will not error if these are omitted, so we throw an error
				// if any of these are empty
				for _, sk := range []string{"s3_access_key", "s3_secret_key"} {
					if sf[sk].(string) == "" {
						return fmt.Errorf("[ERR] No %s found for S3 Log stream setup for Service (%s)", sk, d.Id())
					}
				}

				opts := gofastly.CreateS3Input{
					Service:           d.Id(),
					Version:           latestVersion,
					Name:              sf["name"].(string),
					BucketName:        sf["bucket_name"].(string),
					AccessKey:         sf["s3_access_key"].(string),
					SecretKey:         sf["s3_secret_key"].(string),
					Period:            uint(sf["period"].(int)),
					GzipLevel:         uint(sf["gzip_level"].(int)),
					Domain:            sf["domain"].(string),
					Path:              sf["path"].(string),
					Format:            sf["format"].(string),
					FormatVersion:     uint(sf["format_version"].(int)),
					TimestampFormat:   sf["timestamp_format"].(string),
					ResponseCondition: sf["response_condition"].(string),
				}

				log.Printf("[DEBUG] Create S3 Logging Opts: %#v", opts)
				_, err := conn.CreateS3(&opts)
				if err != nil {
					return err
				}
			}
		}

		// find difference in Papertrail
		if d.HasChange("papertrail") {
			os, ns := d.GetChange("papertrail")
			if os == nil {
				os = new(schema.Set)
			}
			if ns == nil {
				ns = new(schema.Set)
			}

			oss := os.(*schema.Set)
			nss := ns.(*schema.Set)
			removePapertrail := oss.Difference(nss).List()
			addPapertrail := nss.Difference(oss).List()

			// DELETE old papertrail configurations
			for _, pRaw := range removePapertrail {
				pf := pRaw.(map[string]interface{})
				opts := gofastly.DeletePapertrailInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    pf["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Papertrail removal opts: %#v", opts)
				err := conn.DeletePapertrail(&opts)
				if err != nil {
					return err
				}
			}

			// POST new/updated Papertrail
			for _, pRaw := range addPapertrail {
				pf := pRaw.(map[string]interface{})

				opts := gofastly.CreatePapertrailInput{
					Service:           d.Id(),
					Version:           latestVersion,
					Name:              pf["name"].(string),
					Address:           pf["address"].(string),
					Port:              uint(pf["port"].(int)),
					Format:            pf["format"].(string),
					ResponseCondition: pf["response_condition"].(string),
				}

				log.Printf("[DEBUG] Create Papertrail Opts: %#v", opts)
				_, err := conn.CreatePapertrail(&opts)
				if err != nil {
					return err
				}
			}
		}

		// find difference in Sumologic
		if d.HasChange("sumologic") {
			os, ns := d.GetChange("sumologic")
			if os == nil {
				os = new(schema.Set)
			}
			if ns == nil {
				ns = new(schema.Set)
			}

			oss := os.(*schema.Set)
			nss := ns.(*schema.Set)
			removeSumologic := oss.Difference(nss).List()
			addSumologic := nss.Difference(oss).List()

			// DELETE old sumologic configurations
			for _, pRaw := range removeSumologic {
				sf := pRaw.(map[string]interface{})
				opts := gofastly.DeleteSumologicInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    sf["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Sumologic removal opts: %#v", opts)
				err := conn.DeleteSumologic(&opts)
				if err != nil {
					return err
				}
			}

			// POST new/updated Sumologic
			for _, pRaw := range addSumologic {
				sf := pRaw.(map[string]interface{})
				opts := gofastly.CreateSumologicInput{
					Service:           d.Id(),
					Version:           latestVersion,
					Name:              sf["name"].(string),
					URL:               sf["url"].(string),
					Format:            sf["format"].(string),
					FormatVersion:     sf["format_version"].(int),
					ResponseCondition: sf["response_condition"].(string),
					MessageType:       sf["message_type"].(string),
				}

				log.Printf("[DEBUG] Create Sumologic Opts: %#v", opts)
				_, err := conn.CreateSumologic(&opts)
				if err != nil {
					return err
				}
			}
		}

		// find difference in gcslogging
		if d.HasChange("gcslogging") {
			os, ns := d.GetChange("gcslogging")
			if os == nil {
				os = new(schema.Set)
			}
			if ns == nil {
				ns = new(schema.Set)
			}

			oss := os.(*schema.Set)
			nss := ns.(*schema.Set)
			removeGcslogging := oss.Difference(nss).List()
			addGcslogging := nss.Difference(oss).List()

			// DELETE old gcslogging configurations
			for _, pRaw := range removeGcslogging {
				sf := pRaw.(map[string]interface{})
				opts := gofastly.DeleteGCSInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    sf["name"].(string),
				}

				log.Printf("[DEBUG] Fastly gcslogging removal opts: %#v", opts)
				err := conn.DeleteGCS(&opts)
				if err != nil {
					return err
				}
			}

			// POST new/updated gcslogging
			for _, pRaw := range addGcslogging {
				sf := pRaw.(map[string]interface{})
				opts := gofastly.CreateGCSInput{
					Service:           d.Id(),
					Version:           latestVersion,
					Name:              sf["name"].(string),
					User:              sf["email"].(string),
					Bucket:            sf["bucket_name"].(string),
					SecretKey:         sf["secret_key"].(string),
					Format:            sf["format"].(string),
					ResponseCondition: sf["response_condition"].(string),
				}

				log.Printf("[DEBUG] Create GCS Opts: %#v", opts)
				_, err := conn.CreateGCS(&opts)
				if err != nil {
					return err
				}
			}
		}

		// find difference in Response Object
		if d.HasChange("response_object") {
			or, nr := d.GetChange("response_object")
			if or == nil {
				or = new(schema.Set)
			}
			if nr == nil {
				nr = new(schema.Set)
			}

			ors := or.(*schema.Set)
			nrs := nr.(*schema.Set)
			removeResponseObject := ors.Difference(nrs).List()
			addResponseObject := nrs.Difference(ors).List()

			// DELETE old response object configurations
			for _, rRaw := range removeResponseObject {
				rf := rRaw.(map[string]interface{})
				opts := gofastly.DeleteResponseObjectInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    rf["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Response Object removal opts: %#v", opts)
				err := conn.DeleteResponseObject(&opts)
				if err != nil {
					return err
				}
			}

			// POST new/updated Response Object
			for _, rRaw := range addResponseObject {
				rf := rRaw.(map[string]interface{})

				opts := gofastly.CreateResponseObjectInput{
					Service:          d.Id(),
					Version:          latestVersion,
					Name:             rf["name"].(string),
					Status:           uint(rf["status"].(int)),
					Response:         rf["response"].(string),
					Content:          rf["content"].(string),
					ContentType:      rf["content_type"].(string),
					RequestCondition: rf["request_condition"].(string),
					CacheCondition:   rf["cache_condition"].(string),
				}

				log.Printf("[DEBUG] Create Response Object Opts: %#v", opts)
				_, err := conn.CreateResponseObject(&opts)
				if err != nil {
					return err
				}
			}
		}

		// find difference in request settings
		if d.HasChange("request_setting") {
			os, ns := d.GetChange("request_setting")
			if os == nil {
				os = new(schema.Set)
			}
			if ns == nil {
				ns = new(schema.Set)
			}

			ors := os.(*schema.Set)
			nrs := ns.(*schema.Set)
			removeRequestSettings := ors.Difference(nrs).List()
			addRequestSettings := nrs.Difference(ors).List()

			// DELETE old Request Settings configurations
			for _, sRaw := range removeRequestSettings {
				sf := sRaw.(map[string]interface{})
				opts := gofastly.DeleteRequestSettingInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    sf["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Request Setting removal opts: %#v", opts)
				err := conn.DeleteRequestSetting(&opts)
				if err != nil {
					return err
				}
			}

			// POST new/updated Request Setting
			for _, sRaw := range addRequestSettings {
				opts, err := buildRequestSetting(sRaw.(map[string]interface{}))
				if err != nil {
					log.Printf("[DEBUG] Error building Requset Setting: %s", err)
					return err
				}
				opts.Service = d.Id()
				opts.Version = latestVersion

				log.Printf("[DEBUG] Create Request Setting Opts: %#v", opts)
				_, err = conn.CreateRequestSetting(opts)
				if err != nil {
					return err
				}
			}
		}

		// Find differences in VCLs
		if d.HasChange("vcl") {
			// Note: as above with Gzip and S3 logging, we don't utilize the PUT
			// endpoint to update a VCL, we simply destroy it and create a new one.
			oldVCLVal, newVCLVal := d.GetChange("vcl")
			if oldVCLVal == nil {
				oldVCLVal = new(schema.Set)
			}
			if newVCLVal == nil {
				newVCLVal = new(schema.Set)
			}

			oldVCLSet := oldVCLVal.(*schema.Set)
			newVCLSet := newVCLVal.(*schema.Set)

			remove := oldVCLSet.Difference(newVCLSet).List()
			add := newVCLSet.Difference(oldVCLSet).List()

			// Delete removed VCL configurations
			for _, dRaw := range remove {
				df := dRaw.(map[string]interface{})
				opts := gofastly.DeleteVCLInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    df["name"].(string),
				}

				log.Printf("[DEBUG] Fastly VCL Removal opts: %#v", opts)
				err := conn.DeleteVCL(&opts)
				if err != nil {
					return err
				}
			}
			// POST new VCL configurations
			for _, dRaw := range add {
				df := dRaw.(map[string]interface{})
				opts := gofastly.CreateVCLInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    df["name"].(string),
					Content: df["content"].(string),
				}

				log.Printf("[DEBUG] Fastly VCL Addition opts: %#v", opts)
				_, err := conn.CreateVCL(&opts)
				if err != nil {
					return err
				}

				// if this new VCL is the main
				if df["main"].(bool) {
					opts := gofastly.ActivateVCLInput{
						Service: d.Id(),
						Version: latestVersion,
						Name:    df["name"].(string),
					}
					log.Printf("[DEBUG] Fastly VCL activation opts: %#v", opts)
					_, err := conn.ActivateVCL(&opts)
					if err != nil {
						return err
					}

				}
			}
		}

		// Find differences in Cache Settings
		if d.HasChange("cache_setting") {
			oc, nc := d.GetChange("cache_setting")
			if oc == nil {
				oc = new(schema.Set)
			}
			if nc == nil {
				nc = new(schema.Set)
			}

			ocs := oc.(*schema.Set)
			ncs := nc.(*schema.Set)

			remove := ocs.Difference(ncs).List()
			add := ncs.Difference(ocs).List()

			// Delete removed Cache Settings
			for _, dRaw := range remove {
				df := dRaw.(map[string]interface{})
				opts := gofastly.DeleteCacheSettingInput{
					Service: d.Id(),
					Version: latestVersion,
					Name:    df["name"].(string),
				}

				log.Printf("[DEBUG] Fastly Cache Settings removal opts: %#v", opts)
				err := conn.DeleteCacheSetting(&opts)
				if err != nil {
					return err
				}
			}

			// POST new Cache Settings
			for _, dRaw := range add {
				opts, err := buildCacheSetting(dRaw.(map[string]interface{}))
				if err != nil {
					log.Printf("[DEBUG] Error building Cache Setting: %s", err)
					return err
				}
				opts.Service = d.Id()
				opts.Version = latestVersion

				log.Printf("[DEBUG] Fastly Cache Settings Addition opts: %#v", opts)
				_, err = conn.CreateCacheSetting(opts)
				if err != nil {
					return err
				}
			}
		}

		// validate version
		log.Printf("[DEBUG] Validating Fastly Service (%s), Version (%v)", d.Id(), latestVersion)
		valid, msg, err := conn.ValidateVersion(&gofastly.ValidateVersionInput{
			Service: d.Id(),
			Version: latestVersion,
		})

		if err != nil {
			return fmt.Errorf("[ERR] Error checking validation: %s", err)
		}

		if !valid {
			return fmt.Errorf("[ERR] Invalid configuration for Fastly Service (%s): %s", d.Id(), msg)
		}

		log.Printf("[DEBUG] Activating Fastly Service (%s), Version (%v)", d.Id(), latestVersion)
		_, err = conn.ActivateVersion(&gofastly.ActivateVersionInput{
			Service: d.Id(),
			Version: latestVersion,
		})
		if err != nil {
			return fmt.Errorf("[ERR] Error activating version (%d): %s", latestVersion, err)
		}

		// Only if the version is valid and activated do we set the active_version.
		// This prevents us from getting stuck in cloning an invalid version
		d.Set("active_version", latestVersion)
	}

	return resourceServiceV1Read(d, meta)
}
