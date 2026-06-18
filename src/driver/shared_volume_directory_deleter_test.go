package driver

import (
	"fmt"
	"testing"

	"github.com/quobyte/quobyte-csi-driver/mocks"
	"go.uber.org/mock/gomock"
)

func TestDeletionWorker(t *testing.T) {
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)

	mountPath := "/client/mount/"
	node := "testNode1"
	mounter.EXPECT().RemoveAll(mountPath + fmt.Sprintf(DELETE_MARKER_FORMAT, node, "test")).Return(nil)
	mounter.EXPECT().RemoveAll(mountPath + fmt.Sprintf(DELETE_MARKER_FORMAT, node, "test1")).Return(nil)
	channel := make(chan string)
	go deletionWorker(node, mountPath, channel, mounter)
	channel <- mountPath + fmt.Sprintf(DELETE_MARKER_FORMAT, node, "test")
	channel <- mountPath + fmt.Sprintf(DELETE_MARKER_FORMAT, node, "test1")
	channel <- mountPath + fmt.Sprintf(DELETE_MARKER_FORMAT, "someOtherNode", "test")
	close(channel)
}
