package fastly

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
)

func resourceServiceV1() *schema.Resource {
	return &schema.Resource{
		Create: resourceServiceV1Create,
		Read:   resourceServiceV1Read,
		Update: resourceServiceV1Update,
		Delete: resourceServiceV1Delete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Unique name for this Service",
			},

			// Active Version represents the currently activated version in Fastly. In
			// Terraform, we abstract this number away from the users and manage
			// creating and activating. It's used internally, but also exported for
			// users to see.
			"active_version": {
				Type:     schema.TypeInt,
				Computed: true,
			},

			"domain": {
				Type:     schema.TypeSet,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The domain that this Service will respond to",
						},

						"comment": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},

			"condition": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Required: true,
						},
						"statement": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The statement used to determine if the condition is met",
							StateFunc: func(v interface{}) string {
								value := v.(string)
								// Trim newlines and spaces, to match Fastly API
								return strings.TrimSpace(value)
							},
						},
						"priority": {
							Type:        schema.TypeInt,
							Required:    true,
							Description: "A number used to determine the order in which multiple conditions execute. Lower numbers execute first",
						},
						"type": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Type of the condition, either `REQUEST`, `RESPONSE`, or `CACHE`",
						},
					},
				},
			},

			"default_ttl": {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     3600,
				Description: "The default Time-to-live (TTL) for the version",
			},

			"default_host": {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "The default hostname for the version",
			},

			"healthcheck": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// required fields
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "A name to refer to this healthcheck",
						},
						"host": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Which host to check",
						},
						"path": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The path to check",
						},
						// optional fields
						"check_interval": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     5000,
							Description: "How often to run the healthcheck in milliseconds",
						},
						"expected_response": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     200,
							Description: "The status code expected from the host",
						},
						"http_version": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "1.1",
							Description: "Whether to use version 1.0 or 1.1 HTTP",
						},
						"initial": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     2,
							Description: "When loading a config, the initial number of probes to be seen as OK",
						},
						"method": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "HEAD",
							Description: "Which HTTP method to use",
						},
						"threshold": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     3,
							Description: "How many healthchecks must succeed to be considered healthy",
						},
						"timeout": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     500,
							Description: "Timeout in milliseconds",
						},
						"window": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     5,
							Description: "The number of most recent healthcheck queries to keep for this healthcheck",
						},
					},
				},
			},

			"backend": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// required fields
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "A name for this Backend",
						},
						"address": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "An IPv4, hostname, or IPv6 address for the Backend",
						},
						// Optional fields, defaults where they exist
						"auto_loadbalance": {
							Type:        schema.TypeBool,
							Optional:    true,
							Default:     true,
							Description: "Should this Backend be load balanced",
						},
						"between_bytes_timeout": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     10000,
							Description: "How long to wait between bytes in milliseconds",
						},
						"connect_timeout": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     1000,
							Description: "How long to wait for a timeout in milliseconds",
						},
						"error_threshold": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     0,
							Description: "Number of errors to allow before the Backend is marked as down",
						},
						"first_byte_timeout": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     15000,
							Description: "How long to wait for the first bytes in milliseconds",
						},
						"healthcheck": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "The healthcheck name that should be used for this Backend",
						},
						"max_conn": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     200,
							Description: "Maximum number of connections for this Backend",
						},
						"port": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     80,
							Description: "The port number Backend responds on. Default 80",
						},
						"request_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Name of a condition, which if met, will select this backend during a request.",
						},
						"shield": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "The POP of the shield designated to reduce inbound load.",
						},
						"ssl_check_cert": {
							Type:        schema.TypeBool,
							Optional:    true,
							Default:     true,
							Description: "Be strict on checking SSL certs",
						},
						"ssl_hostname": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "SSL certificate hostname",
							Deprecated:  "Use ssl_cert_hostname and ssl_sni_hostname instead.",
						},
						"ssl_cert_hostname": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "SSL certificate hostname for cert verification",
						},
						"ssl_sni_hostname": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "SSL certificate hostname for SNI verification",
						},
						// UseSSL is something we want to support in the future, but
						// requires SSL setup we don't yet have
						// TODO: Provide all SSL fields from https://docs.fastly.com/api/config#backend
						// "use_ssl": &schema.Schema{
						// 	Type:        schema.TypeBool,
						// 	Optional:    true,
						// 	Default:     false,
						// 	Description: "Whether or not to use SSL to reach the Backend",
						// },
						"weight": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     100,
							Description: "The portion of traffic to send to a specific origins. Each origin receives weight/total of the traffic.",
						},
					},
				},
			},

			"force_destroy": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"cache_setting": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// required fields
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "A name to refer to this Cache Setting",
						},
						"action": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Action to take",
						},
						// optional
						"cache_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Name of a condition to check if this Cache Setting applies",
						},
						"stale_ttl": {
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "Max 'Time To Live' for stale (unreachable) objects.",
							Default:     300,
						},
						"ttl": {
							Type:        schema.TypeInt,
							Optional:    true,
							Description: "The 'Time To Live' for the object",
						},
					},
				},
			},

			"gzip": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// required fields
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "A name to refer to this gzip condition",
						},
						// optional fields
						"content_types": {
							Type:        schema.TypeSet,
							Optional:    true,
							Description: "Content types to apply automatic gzip to",
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
						"extensions": {
							Type:        schema.TypeSet,
							Optional:    true,
							Description: "File extensions to apply automatic gzip to. Do not include '.'",
							Elem:        &schema.Schema{Type: schema.TypeString},
						},
						"cache_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Name of a condition controlling when this gzip configuration applies.",
						},
					},
				},
			},

			"header": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// required fields
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "A name to refer to this Header object",
						},
						"action": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "One of set, append, delete, regex, or regex_repeat",
							ValidateFunc: func(v interface{}, k string) (ws []string, es []error) {
								var found bool
								for _, t := range []string{"set", "append", "delete", "regex", "regex_repeat"} {
									if v.(string) == t {
										found = true
									}
								}
								if !found {
									es = append(es, fmt.Errorf(
										"Fastly Header action is case sensitive and must be one of 'set', 'append', 'delete', 'regex', or 'regex_repeat'; found: %s", v.(string)))
								}
								return
							},
						},
						"type": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Type to manipulate: request, fetch, cache, response",
							ValidateFunc: func(v interface{}, k string) (ws []string, es []error) {
								var found bool
								for _, t := range []string{"request", "fetch", "cache", "response"} {
									if v.(string) == t {
										found = true
									}
								}
								if !found {
									es = append(es, fmt.Errorf(
										"Fastly Header type is case sensitive and must be one of 'request', 'fetch', 'cache', or 'response'; found: %s", v.(string)))
								}
								return
							},
						},
						"destination": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Header this affects",
						},
						// Optional fields, defaults where they exist
						"ignore_if_set": {
							Type:        schema.TypeBool,
							Optional:    true,
							Default:     false,
							Description: "Don't add the header if it is already. (Only applies to 'set' action.). Default `false`",
						},
						"source": {
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							Description: "Variable to be used as a source for the header content (Does not apply to 'delete' action.)",
						},
						"regex": {
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							Description: "Regular expression to use (Only applies to 'regex' and 'regex_repeat' actions.)",
						},
						"substitution": {
							Type:        schema.TypeString,
							Optional:    true,
							Computed:    true,
							Description: "Value to substitute in place of regular expression. (Only applies to 'regex' and 'regex_repeat'.)",
						},
						"priority": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     100,
							Description: "Lower priorities execute first. (Default: 100.)",
						},
						"request_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Optional name of a request condition to apply.",
						},
						"cache_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Optional name of a cache condition to apply.",
						},
						"response_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Optional name of a response condition to apply.",
						},
					},
				},
			},

			"s3logging": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// Required fields
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Unique name to refer to this logging setup",
						},
						"bucket_name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "S3 Bucket name to store logs in",
						},
						"s3_access_key": {
							Type:        schema.TypeString,
							Optional:    true,
							DefaultFunc: schema.EnvDefaultFunc("FASTLY_S3_ACCESS_KEY", ""),
							Description: "AWS Access Key",
							Sensitive:   true,
						},
						"s3_secret_key": {
							Type:        schema.TypeString,
							Optional:    true,
							DefaultFunc: schema.EnvDefaultFunc("FASTLY_S3_SECRET_KEY", ""),
							Description: "AWS Secret Key",
							Sensitive:   true,
						},
						// Optional fields
						"path": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Path to store the files. Must end with a trailing slash",
						},
						"domain": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Bucket endpoint",
						},
						"gzip_level": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     0,
							Description: "Gzip Compression level",
						},
						"period": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     3600,
							Description: "How frequently the logs should be transferred, in seconds (Default 3600)",
						},
						"format": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "%h %l %u %t %r %>s",
							Description: "Apache-style string or VCL variables to use for log formatting",
						},
						"format_version": {
							Type:         schema.TypeInt,
							Optional:     true,
							Default:      1,
							Description:  "The version of the custom logging format used for the configured endpoint. Can be either 1 or 2. (Default: 1)",
							ValidateFunc: validateLoggingFormatVersion,
						},
						"timestamp_format": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "%Y-%m-%dT%H:%M:%S.000",
							Description: "specified timestamp formatting (default `%Y-%m-%dT%H:%M:%S.000`)",
						},
						"response_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Name of a condition to apply this logging.",
						},
					},
				},
			},

			"papertrail": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// Required fields
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Unique name to refer to this logging setup",
						},
						"address": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The address of the papertrail service",
						},
						"port": {
							Type:        schema.TypeInt,
							Required:    true,
							Description: "The port of the papertrail service",
						},
						// Optional fields
						"format": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "%h %l %u %t %r %>s",
							Description: "Apache-style string or VCL variables to use for log formatting",
						},
						"response_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Name of a condition to apply this logging",
						},
					},
				},
			},
			"sumologic": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// Required fields
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Unique name to refer to this logging setup",
						},
						"url": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The URL to POST to.",
						},
						// Optional fields
						"format": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "%h %l %u %t %r %>s",
							Description: "Apache-style string or VCL variables to use for log formatting",
						},
						"format_version": {
							Type:         schema.TypeInt,
							Optional:     true,
							Default:      1,
							Description:  "The version of the custom logging format used for the configured endpoint. Can be either 1 or 2. (Default: 1)",
							ValidateFunc: validateLoggingFormatVersion,
						},
						"response_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Name of a condition to apply this logging.",
						},
						"message_type": {
							Type:         schema.TypeString,
							Optional:     true,
							Default:      "classic",
							Description:  "How the message should be formatted.",
							ValidateFunc: validateLoggingMessageType,
						},
					},
				},
			},

			"gcslogging": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// Required fields
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Unique name to refer to this logging setup",
						},
						"email": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The email address associated with the target GCS bucket on your account.",
						},
						"bucket_name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The name of the bucket in which to store the logs.",
						},
						"secret_key": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The secret key associated with the target gcs bucket on your account.",
							Sensitive:   true,
						},
						// Optional fields
						"path": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Path to store the files. Must end with a trailing slash",
						},
						"gzip_level": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     0,
							Description: "Gzip Compression level",
						},
						"period": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     3600,
							Description: "How frequently the logs should be transferred, in seconds (Default 3600)",
						},
						"format": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "%h %l %u %t %r %>s",
							Description: "Apache-style string or VCL variables to use for log formatting",
						},
						"timestamp_format": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "%Y-%m-%dT%H:%M:%S.000",
							Description: "specified timestamp formatting (default `%Y-%m-%dT%H:%M:%S.000`)",
						},
						"response_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Name of a condition to apply this logging.",
						},
					},
				},
			},

			"response_object": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// Required
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Unique name to refer to this request object",
						},
						// Optional fields
						"status": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     200,
							Description: "The HTTP Status Code of the object",
						},
						"response": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "OK",
							Description: "The HTTP Response of the object",
						},
						"content": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "The content to deliver for the response object",
						},
						"content_type": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "The MIME type of the content",
						},
						"request_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Name of the condition to be checked during the request phase to see if the object should be delivered",
						},
						"cache_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Name of the condition checked after we have retrieved an object. If the condition passes then deliver this Request Object instead.",
						},
					},
				},
			},

			"request_setting": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						// Required fields
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Unique name to refer to this Request Setting",
						},
						// Optional fields
						"request_condition": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "",
							Description: "Name of a request condition to apply. If there is no condition this setting will always be applied.",
						},
						"max_stale_age": {
							Type:        schema.TypeInt,
							Optional:    true,
							Default:     60,
							Description: "How old an object is allowed to be, in seconds. Default `60`",
						},
						"force_miss": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Force a cache miss for the request",
						},
						"force_ssl": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Forces the request use SSL",
						},
						"action": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Allows you to terminate request handling and immediately perform an action",
						},
						"bypass_busy_wait": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Disable collapsed forwarding",
						},
						"hash_keys": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Comma separated list of varnish request object fields that should be in the hash key",
						},
						"xff": {
							Type:        schema.TypeString,
							Optional:    true,
							Default:     "append",
							Description: "X-Forwarded-For options",
						},
						"timer_support": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Injects the X-Timer info into the request",
						},
						"geo_headers": {
							Type:        schema.TypeBool,
							Optional:    true,
							Description: "Inject Fastly-Geo-Country, Fastly-Geo-City, and Fastly-Geo-Region",
						},
						"default_host": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "the host header",
						},
					},
				},
			},
			"vcl": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "A name to refer to this VCL configuration",
						},
						"content": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The contents of this VCL configuration",
							StateFunc: func(v interface{}) string {
								switch v.(type) {
								case string:
									hash := sha1.Sum([]byte(v.(string)))
									return hex.EncodeToString(hash[:])
								default:
									return ""
								}
							},
						},
						"main": {
							Type:        schema.TypeBool,
							Optional:    true,
							Default:     false,
							Description: "Should this VCL configuration be the main configuration",
						},
					},
				},
			},
		},
	}
}
