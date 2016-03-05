package monitoring

type MonitoringHelperInterface interface {
	GetResourceToCollectionNameMapper() map[string]string
}
