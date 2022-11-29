package main

import (
	"fmt"
	"os"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

type sysWatcher struct {
	cpuStatInterval int
	cpuThreshold    float64
	memThreshold    float64
	diskThreshold   map[string]float64
	dirThreshold    map[string]int64
}

type sysStats struct {
	cpuUsagePercentage  float64
	memUsagePercentage  float64
	diskUsagePercentage map[string]float64
	directorySize       map[string]int64
}

func (sw *sysWatcher) watchSystemResources() []string {
	diskMounts := []string{}
	dirList := []string{}
	cpuStatInterval := 1
	messages := []string{}

	for diskMount := range sw.diskThreshold {
		diskMounts = append(diskMounts, diskMount)
	}

	for directory := range sw.dirThreshold {
		dirList = append(dirList, directory)
	}

	if sw.cpuStatInterval > 0 {
		cpuStatInterval = sw.cpuStatInterval
	}

	currentStat := sysMetrics(cpuStatInterval, diskMounts, dirList)

	if sw.cpuThreshold != 0 && currentStat.cpuUsagePercentage >= sw.cpuThreshold {
		messages = append(messages, fmt.Sprintf("CPU utilisation reached '%.2f%%'. Current threshold value: '%.2f%%'\n", currentStat.cpuUsagePercentage, sw.cpuThreshold))
	}

	if sw.memThreshold != 0 && currentStat.memUsagePercentage >= sw.memThreshold {
		messages = append(messages, fmt.Sprintf("Memory utilisation reached '%.2f%%'. Current threshold value: '%.2f%%'\n", currentStat.memUsagePercentage, sw.memThreshold))
	}

	for diskMount, threshold := range sw.diskThreshold {
		if currentUsage, ok := currentStat.diskUsagePercentage[diskMount]; ok {
			if threshold != 0 && currentUsage >= threshold {
				messages = append(messages, fmt.Sprintf("Disk utilisation reached '%.2f%%' for the mount point '%s'. Current threshold value: '%.2f%%'\n", currentUsage, diskMount, threshold))
			}
		}
	}

	for directoryPath, threshold := range sw.dirThreshold {
		if currentUsage, ok := currentStat.directorySize[directoryPath]; ok {
			if threshold != 0 && currentUsage >= threshold {
				messages = append(messages, fmt.Sprintf("Directory Size Reached Threshold of '%d' bytes for the path '%s'. Current threshold value: '%d'\n", currentUsage, directoryPath, threshold))
			}
		}
	}

	return messages
}

func sysMetrics(cpuStatInterval int, diskMounts, dirList []string) sysStats {
	//
	result := sysStats{}

	result.fetchCPUStats(cpuStatInterval)

	result.fetchMemStats()

	if len(diskMounts) > 0 {
		result.fetchDiskStats(diskMounts)
	}

	if len(dirList) > 0 {
		result.fetchDirSize(dirList)
	}

	return result
}

func (s *sysStats) fetchCPUStats(interval int) {
	cpuUsage, err := cpu.Percent(time.Duration(interval)*time.Second, false)
	if err != nil {
		fmt.Println("ERROR: Cannot fetch CPU stats. Because: ", err.Error())
		return
	}

	lenOfUsage := len(cpuUsage)
	if lenOfUsage == 1 {
		s.cpuUsagePercentage = cpuUsage[0]
		return
	}
	fmt.Printf("ERROR: Unexpected CPU usage length of '%d' detected\n", lenOfUsage)
}

func (s *sysStats) fetchMemStats() {
	vMemory, err := mem.VirtualMemory()
	if err != nil {
		fmt.Println("ERROR: Cannot fetch memory stats. Because: ", err.Error())
	}
	s.memUsagePercentage = vMemory.UsedPercent
}

func (s *sysStats) fetchDiskStats(mountPoints []string) {
	result := map[string]float64{}

	partitions, err := disk.Partitions(true)
	if err != nil {
		fmt.Println("ERROR: Cannot fetch disk partitions. Because: ", err.Error())
		return
	}

	partitionMap := map[string]float64{}
	for _, partition := range partitions {
		usage, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			fmt.Printf("ERROR: Cannot fetch disk usage for the path '%s'. Because: %s\n", partition.Mountpoint, err.Error())
		} else {
			partitionMap[partition.Mountpoint] = usage.UsedPercent
		}
	}

	for _, mountPoint := range mountPoints {
		if usage, ok := partitionMap[mountPoint]; ok {
			result[mountPoint] = usage
		}
	}

	s.diskUsagePercentage = result
}

func (s *sysStats) fetchDirSize(directories []string) {
	result := map[string]int64{}

	for _, directory := range directories {
		info, err := os.Lstat(directory)
		if err == nil {
			dirSize := getDirSize(directory, info)
			result[directory] = dirSize
		} else {
			fmt.Printf("ERROR: Cannot fetch directory size of the path '%s'. Because: %s\n", directory, err.Error())
		}
	}

	s.directorySize = result
}

func getDirSize(currentPath string, info os.FileInfo) int64 {
	size := info.Size()
	if !info.IsDir() {
		return size
	}

	dir, err := os.Open(currentPath)
	if err != nil {
		// Meaning we're unable to access/open the file
		return size
	}
	defer dir.Close()

	fis, err := dir.Readdir(-1)
	if err == nil {
		for _, fi := range fis {
			if fi.Name() == "." || fi.Name() == ".." {
				continue
			}
			size += getDirSize(currentPath+"/"+fi.Name(), fi)
		}
	} else {
		fmt.Printf("ERROR: Cannot read the directory at path: '%s'. Because: %s\n", currentPath, err.Error())
	}

	return size
}
