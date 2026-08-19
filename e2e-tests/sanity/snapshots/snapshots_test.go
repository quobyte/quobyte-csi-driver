package snapshots

import "testing"

// TestSnapshotRestoresVolumeContent covers the "Snapshot tests" entry of the TODO list in
// e2e-tests/sanity/README.md.
//
// Quobyte does not create a volume per snapshot, so the driver's snapshot path is unusual:
// restoring a snapshot produces a dummy PV whose handle carries the
// SnapshotVolumeHandlePrefix and whose deletion is a no-op, because deleting the
// VolumeSnapshot is what removes the snapshot (see createSnapshotResponse and DeleteVolume
// in src/driver/controller.go). That asymmetry is the part most worth pinning down.
//
// To implement:
//   - the external-snapshotter CRDs and controller have to be installed; test_runner does
//     that automatically for an environment that enables snapshots
//     (deploy_snapshot_resources_if_needed), so this test only needs to create the
//     VolumeSnapshotClass and the VolumeSnapshot objects
//   - those are custom resources, so the test needs the snapshot API types -- add
//     github.com/kubernetes-csi/external-snapshotter/client/v8 to e2e-tests/go.mod, or
//     drive them untyped through a dynamic client
//   - provision a PVC, write a known file, take a VolumeSnapshot of it and wait for
//     readyToUse
//   - restore into a second PVC with the snapshot as its dataSource, mount it and assert
//     the file is there with the content written before the snapshot
//   - assert that changes made after the snapshot are absent from the restored volume
//   - tear down and assert deleting the VolumeSnapshot removes the snapshot in Quobyte,
//     while deleting the restored PV alone does not
//
// Not implemented yet.
func TestSnapshotRestoresVolumeContent(t *testing.T) {
	t.Skip("not implemented yet -- see the TODO list in e2e-tests/sanity/README.md")
}
