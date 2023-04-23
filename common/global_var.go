package common

const (
	PidDir           = "/home/fdfs"
	PidFile          = PidDir + "/fdfs_manager.pid"
	ConfigDir        = "/etc/fdfs"
	TrackerConfigDir = ConfigDir + "/tracker*.conf"
	StorageConfigDir = ConfigDir + "/storage*.conf"
	TrackerCommand   = "fdfs_trackerd"
	StorageCommand   = "fdfs_storaged"
	CommandBinPath   = "/usr/bin/"
)

const (
	TrackerService = iota
	StorageService
)

type ServiceType int

var (
	ConfigFile = "/etc/fdfs/fdfs_manager.yml"
	AppName    = "fdfs_manager"
)

type ServiceInfo struct {
	ServiceType ServiceType
	ConfigPath  string
	BasePath    string
	Pid         string
}
