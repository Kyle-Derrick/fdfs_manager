package manager

import (
	"fdfs_manager/common"
	"fmt"
	"github.com/go-ini/ini"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type FdfsConfig struct {
	TrackerConfig []string `yaml:"tracker-config"`
	StorageConfig []string `yaml:"storage-config"`
}

var (
	trackerInfos   []*common.ServiceInfo
	storageInfos   []*common.ServiceInfo
	serviceNum     = 0
	createdPidFile = false
	shutdownLock   = sync.Mutex{}
)

func Run(args []string) {
	cmd := "start"
	if len(args) > 0 {
		cmd = args[0]
	}

	defer func() {
		if createdPidFile {
			os.Remove(common.PidFile)
		}
		if serviceNum == 0 {
			log.Println("退出程序")
			return
		}
		log.Println("开始终结服务...")
		err := shutdown()
		if err != nil {
			log.Println("终结服务时出现错误", err)
		}
		if err != nil {
			log.Println("删除pid文件出错", err)
		}
	}()

	switch cmd {
	case "start":
		log.Println("启动fdfs服务...")
		err := start()
		if err != nil {
			panic("启动失败: " + err.Error())
		}
	case "stop":
		log.Println("停止fdfs服务...")
		err := stop()
		if err != nil {
			panic("停止失败: " + err.Error())
		}
	case "restart":
		err := stop()
		if err != nil {
			panic("停止失败: " + err.Error())
		}
		err = start()
		if err != nil {
			panic("启动失败: " + err.Error())
		}
	}

	if serviceNum == 0 {
		return
	}
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		for {
			shutdownLock.Lock()
			err := checkService()
			if err != nil {
				log.Println("服务监测异常", err)
			}
			shutdownLock.Unlock()
			time.Sleep(1 * time.Second)
		}
	}()
	for {
		s := <-signals
		switch s {
		case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			shutdownLock.Lock()
			return
		}
	}
}

func shutdown() error {
	if len(trackerInfos) > 0 {
		for _, info := range trackerInfos {
			err := exec.Command(common.CommandBinPath+common.TrackerCommand, info.ConfigPath, "stop").Run()
			if err != nil {
				log.Println("fdfs tracker 停止失败: ", info.ConfigPath)
			}
		}
	}
	if len(storageInfos) > 0 {
		for _, info := range storageInfos {
			err := exec.Command(common.CommandBinPath+common.StorageCommand, info.ConfigPath, "stop").Run()
			if err != nil {
				log.Println("fdfs storage 停止失败: ", info.ConfigPath)
			}
		}
	}
	return nil
}

func checkService() error {
	if len(trackerInfos) > 0 {
		for _, info := range trackerInfos {
			if len(info.Pid) == 0 || !processExists(info.Pid, info.ConfigPath) {
				log.Println("fdfs tracker 服务挂掉, restarting...: ", info.ConfigPath)
				pid, err := startService(common.TrackerCommand, info.ConfigPath, info.BasePath)
				info.Pid = pid
				if err != nil {
					log.Println("fdfs tracker 服务挂掉后重启失败: ", info.ConfigPath)
				} else {
					log.Println("fdfs tracker 服务重启成功: ", info.ConfigPath)
				}
			}
		}
	}
	if len(storageInfos) > 0 {
		for _, info := range storageInfos {
			if len(info.Pid) == 0 || !processExists(info.Pid, info.ConfigPath) {
				log.Println("fdfs storage 服务挂掉, restarting...: ", info.ConfigPath)
				pid, err := startService(common.StorageCommand, info.ConfigPath, info.BasePath)
				info.Pid = pid
				if err != nil {
					log.Println("fdfs storage服务挂掉后重启失败: ", info.ConfigPath)
				} else {
					log.Println("fdfs storage 服务重启成功: ", info.ConfigPath, "新pid: ", pid)
				}
			}
		}
	}
	return nil
}

func start() error {
	err := createPidFile()
	if err != nil {
		return err
	}
	var conf = FdfsConfig{}
	log.Println("读取配置文件：", common.ConfigFile)
	bytes, err := ioutil.ReadFile(common.ConfigFile)
	if err == nil {
		err = yaml.Unmarshal(bytes, &conf)
		if err != nil {
			log.Println("解析配置失败: ", common.ConfigFile)
		}
	}
	if conf.TrackerConfig == nil || len(conf.TrackerConfig) == 0 {
		conf.TrackerConfig = []string{common.TrackerConfigDir}
	}
	if conf.StorageConfig == nil || len(conf.StorageConfig) == 0 {
		conf.StorageConfig = []string{common.StorageConfigDir}
	}
	log.Println("开始启动fdfs tracker服务...")
	trackerInfos = make([]*common.ServiceInfo, 0, len(conf.TrackerConfig))
	for _, config := range conf.TrackerConfig {
		trackerInfos = getBasePath(common.TrackerService, config, trackerInfos)
	}
	log.Println("开始启动fdfs storage服务...")
	storageInfos = make([]*common.ServiceInfo, 0, len(conf.StorageConfig))
	for _, config := range conf.StorageConfig {
		storageInfos = getBasePath(common.StorageService, config, storageInfos)
	}
	return nil
}

func getBasePath(serviceType common.ServiceType, configPath string, serviceInfos []*common.ServiceInfo) []*common.ServiceInfo {
	log.Println("解析配置路径：", configPath)
	glob, err := filepath.Glob(configPath)
	if err != nil {
		log.Panic("解析fdfs 配置路径失败", configPath)
	}
	for _, config := range glob {
		log.Println("加载配置：", config)
		cfg, err := ini.Load(config)
		if err != nil {
			log.Panic("加载指定fdfs 配置失败", config)
		}
		section := cfg.Section("")
		basePath := section.Key("base_path").String()
		if len(basePath) == 0 {
			log.Panic("无法获取fdfs 的base path配置", config)
		}
		var cmd string
		if serviceType == common.TrackerService {
			cmd = common.TrackerCommand
		} else {
			cmd = common.StorageCommand
		}
		pid, err := startService(cmd, config, basePath)
		if err != nil {
			log.Panic(err)
		}
		serviceInfos = append(serviceInfos, &common.ServiceInfo{
			ServiceType: serviceType,
			ConfigPath:  config,
			BasePath:    basePath,
			Pid:         pid,
		})
		log.Println("已启动", cmd, "服务：", config, ", pid: ", pid)
		serviceNum++
	}
	return serviceInfos
}

func startService(cmd string, config string, basePath string) (string, error) {
	command := exec.Command(common.CommandBinPath+cmd, config, "restart")
	err := command.Run()
	if err != nil {
		exec.Command(common.CommandBinPath+cmd, config, "stop").Run()
		return "", fmt.Errorf("启动fdfs 服务失败: %s", config)
	}
	pidFile := filepath.Join(basePath, "data/"+cmd+".pid")
	var pidByte []byte
	for i := 0; i < 10; i++ {
		time.Sleep(time.Second)
		pidByte, err = ioutil.ReadFile(pidFile)
		if err == nil {
			break
		}
		if i == 10 {
			exec.Command(common.CommandBinPath+cmd, config, "stop").Run()
			return "", fmt.Errorf("获取已启动的fdfs 服务的pid失败: %s : %s \n%s", config, pidFile, err.Error())
		}
	}
	return string(pidByte), nil
}

func stop() error {
	pid, err := checkExists(common.PidFile)
	if err == nil {
		return nil
	}
	command := exec.Command(common.CommandBinPath+"kill", "-15", pid)
	pidNum, err := strconv.Atoi(pid)
	if err != nil {
		return err
	}
	syscall.Kill(pidNum, syscall.SIGTERM)
	err = command.Run()
	if err != nil {
		return err
	}
	var pidTemp string
	for {
		pidTemp, err = checkExists(common.PidFile)
		if pidTemp == "" || pidTemp != pid {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func createPidFile() error {
	_, err := checkExists(common.PidFile)
	if err != nil {
		return err
	}
	err = os.MkdirAll(common.PidDir, os.FileMode(0644))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(common.PidFile, []byte(strconv.Itoa(os.Getpid())), os.FileMode(0644))
	if err != nil {
		return err
	}
	createdPidFile = true
	return nil
}

func checkExists(path string) (string, error) {
	file, err := ioutil.ReadFile(path)
	if err == nil {
		pid := strings.TrimSpace(string(file))
		if processExists(pid, common.AppName) {
			return pid, fmt.Errorf("已存在进程: [%s], pid文件: [%s]", pid, path)
		}
	}
	return "", nil
}

func processExists(pid string, cmd string) bool {
	_, err := strconv.Atoi(pid)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join("/proc", pid))
	if err == nil {
		return true
	}
	command := exec.Command(common.CommandBinPath + "ps -o command -p " + pid + " | grep " + cmd)
	output, err := command.CombinedOutput()
	if err != nil || output == nil || len(output) == 0 {
		return false
	}
	return strings.Contains(string(output), cmd)
}
