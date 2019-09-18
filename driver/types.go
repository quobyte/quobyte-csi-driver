package driver

type ExpandVolumeReq struct {
	volID         string
	expandSecrets map[string]string
	capacity      int64
}
