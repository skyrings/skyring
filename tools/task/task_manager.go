package task

var (
	taskMgr Manager
)

func Initialize() error {
	taskMgr = NewManager()
	return nil
}

func GetManager() *Manager {
	return &taskMgr
}
