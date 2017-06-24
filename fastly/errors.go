package fastly

import (
	"errors"
)

var fastlyNoServiceFoundErr = errors.New("No matching Fastly Service found")
