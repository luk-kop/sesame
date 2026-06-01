package app

const (
	ExitOK                = 0
	ExitRuntimeError      = 1
	ExitUsageError        = 2
	ExitPreflightFailed   = 3
	ExitMissingDependency = 4
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	return e.Err
}
