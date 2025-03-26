package errmsg

import "errors"

var ErrNotRegularFile = errors.New("not a regular file")
var ErrDuplicateEntry = errors.New("duplicate entry")
var ErrPublishFailed = errors.New("publish failed")
