package cardrender

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid_input")
	ErrRenderTimeout       = errors.New("render_timeout")
	ErrBrowserUnavailable  = errors.New("browser_unavailable")
	ErrOSSUploadFailed     = errors.New("oss_upload_failed")
	ErrInvalidTemplate     = errors.New("invalid_template")
	ErrScreenshotEmpty     = errors.New("screenshot_empty")
)

type RenderError struct {
	Category error
	Message  string
	Cause    error
}

func (e *RenderError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *RenderError) Unwrap() error {
	return e.Category
}

func newRenderError(category error, message string, cause error) *RenderError {
	return &RenderError{Category: category, Message: message, Cause: cause}
}
