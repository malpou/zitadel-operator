package controller

// Test-only handles on the pure functions.
//
//nolint:gochecknoglobals // test export shims.
var (
	Classify        = classify
	OIDCConfig      = oidcConfig
	OIDCEqual       = oidcEqual
	SameSet         = sameSet
	SyncSet         = syncSet[string]
	FlowIDs         = flowIDs
	RotateRequested = rotateRequested
	ErrDependency   = errDependencyNotReady
	ErrSecret       = errSecretUnavailable
	Resync          = resync
	DependencyRetry = dependencyRetry
)
