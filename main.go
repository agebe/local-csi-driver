// grok helped creating this code
// also read:
// https://arslan.io/2018/06/21/how-to-write-a-container-storage-interface-csi-plugin/
// https://github.com/kubernetes/community/blob/1a5277642cef37dd83273236ddf93bde67c342e1/contributors/design-proposals/storage/container-storage-interface.md
package main

import (
    "context"
    "fmt"
    "net"
    "os"
    "syscall"
    "io/fs"
    "errors"
    "strconv"
    "github.com/container-storage-interface/spec/lib/go/csi"
    "google.golang.org/grpc"
    "k8s.io/klog/v2"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    "github.com/flytam/filenamify"
)

const (
  driverName = "local.csi.driver"
  driverVersion = "0.1.19"
)

// IdentityServer implements the CSI IdentityServer interface
type IdentityServer struct{
  csi.UnimplementedIdentityServer
}

func (ids *IdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
    klog.Infof("IdentityServer.GetPluginInfo called with request: %v", req)
    return &csi.GetPluginInfoResponse{
        Name:          driverName,
        VendorVersion: driverVersion,
    }, nil
}

func (ns *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
    klog.Infof("NodeServer.NodeGetInfo called with request: %v", req)
    hostname, err := os.Hostname()
    if err != nil {
      return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to get hostname: %v", err))
    }
    return &csi.NodeGetInfoResponse{
        NodeId:            hostname,
    }, nil
}

func (ids *IdentityServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
    klog.Infof("IdentityServer.Probe called with request: %v", req)
    return &csi.ProbeResponse{}, nil
}

// NodeServer implements the CSI NodeServer interface
type NodeServer struct{
  csi.UnimplementedNodeServer
}

func (ns *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
    klog.Infof("NodeGetCapabilities called with request: %v", req)

    return &csi.NodeGetCapabilitiesResponse{
        Capabilities: []*csi.NodeServiceCapability{
            {
                Type: &csi.NodeServiceCapability_Rpc{
                    Rpc: &csi.NodeServiceCapability_RPC{
                        Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
                    },
                },
            },
            // Add more capabilities if your driver supports them
        },
    }, nil
}

// https://stackoverflow.com/a/10510783
func exists(path string) (bool) {
    _, err := os.Stat(path)
    if err == nil {
        return true
    }
    if errors.Is(err, fs.ErrNotExist) {
        return false
    }
    return false
}

func toFileMode(permissionStr string, defaultMode fs.FileMode) (fs.FileMode) {
    // Example string representation of file permissions in octal
    //permissionStr := "0755"
    // Convert the string to an int64, base 8 for octal
    permissionInt, err := strconv.ParseInt(permissionStr, 8, 64)
    if err != nil {
        klog.Infof("Error parsing permission string '%s': %v", permissionStr, err)
        return defaultMode
    }
    // Convert int64 to os.FileMode
    return os.FileMode(permissionInt)
}

func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
    klog.Infof("NodeServer.NodePublishVolume called with request: %v", req)
    targetPath := req.GetTargetPath()
    readonly := req.GetReadonly()
    klog.Infof("NodeServer.NodePublishVolume target path : %v", targetPath)

    volumeID := req.GetVolumeId()
    volumeContext := req.GetVolumeContext()
    dirKey := "directory"
    dirName, dirNameExists := volumeContext[dirKey]
    fileModeKey := "dirmode"
    fileMode, fileModeExists := volumeContext[fileModeKey]
    if dirNameExists {
        klog.Infof("using directory name '%s' from directory context", dirName)
    } else {
        klog.Infof("Key '%s' not found in volume contextm using volumeID '%s' instead", dirKey, volumeID)
        dirName = volumeID
    }
    safeDirName, safeDirNameErr := filenamify.FilenamifyV2(dirName)
    if safeDirNameErr != nil {
      return nil, fmt.Errorf("failed to create safe directory name from '%s': %v", dirName, safeDirNameErr)
    }
    if(dirName != safeDirName) {
      klog.Infof("filenamified directory name to '%s'", safeDirName)
    }
    // hard-coding /mnt/ is no limitation as the mapping to the host filesystem happens in the node server daemonset definition
    storagePath := "/mnt/" + safeDirName
    if exists(storagePath) {
      klog.Infof("storagePath '%s' already exists", storagePath)
    } else {
      klog.Infof("creating storagePath '%s' ...", storagePath)
      if err := os.MkdirAll(storagePath, 0755); err != nil {
        return nil, fmt.Errorf("failed to create directory '%s': %v", storagePath, err)
      }
    }
    if !exists(storagePath) {
      klog.Infof("failed to create storagePath '%s'", storagePath)
      return nil, fmt.Errorf("failed to create directory '%s'", storagePath)
    }
    if(fileModeExists) {
      os.Chmod(storagePath, toFileMode(fileMode, 0755))
    }
    if !exists(targetPath) {
      klog.Infof("target path '%s' does not exist, creating ...", targetPath)
      if err := os.MkdirAll(targetPath, 0755); err != nil {
        return nil, fmt.Errorf("failed to create directory (target) '%s': %v", targetPath, err)
      }
    }
    // Perform bind mount
    mountFlags := syscall.MS_BIND
    if readonly {
      mountFlags |= syscall.MS_RDONLY
    }
    if err := syscall.Mount(storagePath, targetPath, "none", uintptr(mountFlags), ""); err != nil {
      return nil, status.Errorf(codes.Internal, "Failed to mount %s to %s: %v", storagePath, targetPath, err)
    }
    klog.Infof("Bind mounted '%s' to '%s'", storagePath, targetPath)
    return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
    klog.Infof("NodeServer.NodeUnpublishVolume called with request: %v", req)
    volumeID := req.GetVolumeId()
    targetPath := req.GetTargetPath()
    klog.Infof("NodeServer.NodeUnpublishVolume volumeID '%s', target path '%s'", volumeID, targetPath)
    // Basic validation
    if volumeID == "" {
        return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
    }
    if targetPath == "" {
        return nil, status.Error(codes.InvalidArgument, "Target path cannot be empty")
    }
    // Check if the target path exists
    if _, err := os.Stat(targetPath); err != nil {
        if os.IsNotExist(err) {
            // If the directory does not exist, we assume it's already unmounted or was never mounted
            klog.Infof("Target path '%s' does not exist, assuming already unmounted", targetPath)
            return &csi.NodeUnpublishVolumeResponse{}, nil
        }
        return nil, status.Errorf(codes.Internal, "Failed to check target path %s: %v", targetPath, err)
    }
    // Attempt to unmount the bind mount
    if err := syscall.Unmount(targetPath, syscall.MNT_DETACH); err != nil {
        return nil, status.Errorf(codes.Internal, "Failed to unmount %s: %v", targetPath, err)
    }
    klog.Infof("Successfully unmounted volume '%s' from '%s'", volumeID, targetPath)
    return &csi.NodeUnpublishVolumeResponse{}, nil
}

// New method to satisfy the interface
func (ns *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
    return nil, status.Error(codes.Unimplemented, "NodeExpandVolume not implemented")
}

// Main function to set up the server
func main() {
    klog.InitFlags(nil)
    klog.Infof("Starting CSI driver server, local-csi-driver %v", driverVersion)
    socketpath := "/csi/csi.sock"
    syscall.Unlink(socketpath)
    listener, err := net.Listen("unix", socketpath)
    if err != nil {
        klog.Fatalf("Failed to listen: %v", err)
    }
    grpcServer := grpc.NewServer()
    csi.RegisterIdentityServer(grpcServer, &IdentityServer{})
    csi.RegisterNodeServer(grpcServer, &NodeServer{})
    if err := grpcServer.Serve(listener); err != nil {
        klog.Fatalf("Failed to serve: %v", err)
    }
}
