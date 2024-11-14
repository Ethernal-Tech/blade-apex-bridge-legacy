package framework

type TestingT interface {
	Errorf(format string, args ...interface{})
	FailNow()
	Helper()
	Cleanup(f func())
	Logf(format string, args ...any)
	Name() string
	Skipf(format string, args ...any)
}
