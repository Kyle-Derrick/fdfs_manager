package main

import (
	"fdfs_manager/common"
	"fdfs_manager/manager"
	"flag"
	"os"
	"path/filepath"
)

func main() {
	executable, _ := os.Executable()
	_, common.AppName = filepath.Split(executable)
	flag.StringVar(&common.ConfigFile, "config", common.ConfigFile, "指定fdfs管理器的配置文件路径")
	flag.Parse()
	args := flag.Args()
	manager.Run(args)
}
