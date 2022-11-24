package driver

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/klog"
)

// Rename deleted PV of shared volume with with <node_name>_<dir>
const DELETE_MARKER_FORMAT = "%s_delete_%s"

func LaunchDirectoryDeleter(nodeName, mountPath string, sharedVolumes []string, parallelDeletes int) {
	directoryChannel := make(chan string, parallelDeletes)

	go walkAndQueueDirectories(nodeName, mountPath, sharedVolumes, directoryChannel)
	for i := 0; i < parallelDeletes; i++ {
		go deletionWorker(nodeName, mountPath, directoryChannel)
	}
}

func walkAndQueueDirectories(nodeName, mountPath string, sharedVolumes []string, directoryChannel chan<- string) {
	for {
		time.Sleep(5 * time.Minute)
		for _, sharedVolume := range sharedVolumes {
			func() {
				sharedVolume := strings.TrimSpace(sharedVolume)
				if len(sharedVolume) == 0 {
					return
				}
				if volumePath, err := os.Open(filepath.Join(mountPath, sharedVolume)); err == nil {
					files, err := volumePath.Readdirnames(0)
					// could be partial read
					for _, file := range files {
						if strings.HasPrefix(file, nodeName+"_delete_pvc-") { // node name part of DELETE_MARKER_FORMAT
							directoryChannel <- filepath.Join(volumePath.Name(), file)
						}
					}
					defer volumePath.Close()
					if err != nil {
						klog.Infof("Partially read directories from the shared volume %s due to %s", volumePath.Name(), err)
					}
				} else {
					klog.Errorf("Could not read contents of the shared volume %s due to %s", volumePath.Name(), err)
					return
				}
			}()
		}
	}
}

func deletionWorker(nodeName, mountPath string, directoryChannel <-chan string) {
	for directory := range directoryChannel {
		dir, file := filepath.Split(directory)
		if strings.HasPrefix(dir, mountPath) && strings.HasPrefix(file, nodeName) {
			klog.Infof("Deleting directory %s", directory)
			os.RemoveAll(directory)
		}
	}
}
