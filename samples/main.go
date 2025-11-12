/**
 * Copyright 2021 IBM Corp.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package main ...
package main

import (
	"flag"
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/net/context"

	provider_file_util "github.com/IBM/ibmcloud-volume-file-vpc/file/utils"
	vpcfileconfig "github.com/IBM/ibmcloud-volume-file-vpc/file/vpcconfig"
	"github.com/IBM/ibmcloud-volume-interface/config"
	"github.com/IBM/ibmcloud-volume-interface/lib/provider"
	userError "github.com/IBM/ibmcloud-volume-interface/lib/utils"
	"github.com/IBM/ibmcloud-volume-interface/provider/local"
	"github.com/IBM/secret-utils-lib/pkg/k8s_utils"
	utils "github.com/IBM/secret-utils-lib/pkg/utils"
	uid "github.com/gofrs/uuid"
)

var (
	defaultChoice = flag.Int("choice", 0, "Choice")
)

func getContextLogger() (*zap.Logger, zap.AtomicLevel) {
	consoleDebugging := zapcore.Lock(os.Stdout)
	consoleErrors := zapcore.Lock(os.Stderr)
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "ts"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	traceLevel := zap.NewAtomicLevel()
	traceLevel.SetLevel(zap.InfoLevel)
	core := zapcore.NewTee(
		zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), consoleDebugging, zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return (lvl >= traceLevel.Level()) && (lvl < zapcore.ErrorLevel)
		})),
		zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), consoleErrors, zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.ErrorLevel
		})),
	)
	logger := zap.New(core, zap.AddCaller())
	return logger, traceLevel
}

func updateRequestID(err error, requestID string) error {
	if err == nil {
		return err
	}
	usrError, ok := err.(userError.Message)
	if !ok {
		return err
	}
	usrError.RequestID = requestID
	return usrError
}

func main() {
	snapshotID := ""
	SourceVolumeID := ""
	flag.Parse()
	// Setup new style zap logger
	logger, traceLevel := getContextLogger()
	defer logger.Sync()

	// Load config file
	k8sClient, _ := k8s_utils.FakeGetk8sClientSet()
	_ = k8s_utils.FakeCreateSecret(k8sClient, utils.DEFAULT, "./samples/sample-secret-config.toml")
	conf, err := config.ReadConfig(k8sClient, logger)
	if err != nil {
		logger.Fatal("Error loading configuration")
	}

	logger.Info("Global Configuration is================", zap.Reflect("Config", conf))

	// Check if debug log level enabled or not
	if conf.Server != nil && conf.Server.DebugTrace {
		traceLevel.SetLevel(zap.DebugLevel)
	}

	vpcFileConfig := &vpcfileconfig.VPCFileConfig{
		VPCConfig:    conf.VPC,
		ServerConfig: conf.Server,
	}
	// enabling VPC provider as default
	vpcFileConfig.VPCConfig.Enabled = true

	providerRegistry, err := provider_file_util.InitProviders(vpcFileConfig, &k8sClient, logger)

	if err != nil {
		logger.Fatal("Error configuring providers", local.ZapError(err))
	}

	//dc_name := "mex01"
	providerName := conf.Softlayer.SoftlayerBlockProviderName
	if conf.IKS != nil && conf.IKS.Enabled {
		providerName = conf.IKS.IKSBlockProviderName
	} else if conf.Softlayer.SoftlayerFileEnabled {
		providerName = conf.Softlayer.SoftlayerFileProviderName
	} else if conf.VPC.Enabled {
		providerName = conf.VPC.VPCVolumeType
	}

	logger.Info("Provider Name is ================", zap.Reflect("providerName", providerName))

	valid := true
	for valid {
		fmt.Println("\n\nSelect your choice\n 1- Get volume details \n 2- Create snapshot \n 3- list snapshots \n 4- Get snapshot by snapshot name \n 5- Snapshot details \n 6- Snapshot Order \n 7- Create volume from snapshot\n 8- Delete volume \n 9- Delete Snapshot \n 10- List all Snapshot \n 12- Authorize volume \n 13- Create VPC Volume \n 14- Create VPC Snapshot \n 15- Create VPC target \n 16- Delete VPC target \n 17- Get volume by name \n 18- List volumes \n 19- Get volume target \n 20 - Wait for create volume target \n 21 - Wait for delete volume target \n 22 - Expand Volume \n 23 - Get Share Profile\n Your choice?:")

		var choiceN int
		var volumeID, targetID string
		var snapshotName string
		var er11 error
		if *defaultChoice == 0 {
			_, _ = fmt.Scanf("%d", &choiceN)
		} else {
			choiceN = *defaultChoice
		}
		if er11 != nil {
			fmt.Printf("Wrong input, please provide option in int: ")
			fmt.Printf("\n\n")
			continue
		}
		ctxLogger, _ := getContextLogger()
		uuid, _ := uid.NewV4() // #nosec G104: Attempt to randomly generate uuid
		requestID := uuid.String()
		ctxLogger = ctxLogger.With(zap.String("RequestID", requestID))
		ctx := context.WithValue(context.TODO(), provider.RequestID, requestID)
		prov, err := providerRegistry.Get(providerName)
		if err != nil {
			ctxLogger.Error("Not able to get the said provider, might be its not registered", local.ZapError(err))
			continue
		}
		sess, _, err := provider_file_util.OpenProviderSessionWithContext(ctx, prov, vpcFileConfig, providerName, ctxLogger)
		if err != nil {
			ctxLogger.Error("Failed to get session", zap.Reflect("Error", err))
			continue
		}

		defer sess.Close()
		defer ctxLogger.Sync()
		if choiceN == 1 {
			fmt.Println("You selected choice to get volume details")
			fmt.Printf("Please enter volume ID: ")
			_, _ = fmt.Scanf("%s", &volumeID)
			volume, errr := sess.GetVolume(volumeID)
			if errr == nil {
				ctxLogger.Info("SUCCESSFULLY get volume details ================>", zap.Reflect("VolumeDetails", volume))
			} else {
				ctxLogger.Info("Provider error is ================>", zap.Reflect("ErrorType", userError.GetErrorType(errr)))
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED to get volume details ================>", zap.Reflect("VolumeID", volumeID), zap.Reflect("Error", errr))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 2 {
			fmt.Println("You selected choice to create snapshot")
			fmt.Printf("Please enter volume ID: ")
			_, _ = fmt.Scanf("%s", &volumeID)
			fmt.Printf("Please enter snapshot Name: ")
			_, _ = fmt.Scanf("%s", &snapshotName)
			snapshotRequest := provider.SnapshotParameters{}
			snapshotRequest.Name = snapshotName
			tags := make(map[string]string)
			tags["tag1"] = "snapshot-tag1"
			snapshotRequest.SnapshotTags = tags
			snapshot, errr := sess.CreateSnapshot(volumeID, snapshotRequest)
			if errr == nil {
				ctxLogger.Info("Successfully created snapshot on ================>", zap.Reflect("SourceVolumeID", volumeID))
				ctxLogger.Info("Snapshot details: ", zap.Reflect("Snapshot", snapshot))
			} else {
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("Failed to create snapshot on ================>", zap.Reflect("VolumeID", volumeID), zap.Reflect("Error", errr))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 3 {
			fmt.Println("You selected choice to list snapshots from volume")
			tags := map[string]string{}
			SourceVolumeID := ""
			fmt.Printf("Please enter sourceVolumeID to filter snapshots: ")
			_, _ = fmt.Scanf("%s", &SourceVolumeID)
			if SourceVolumeID != "" {
				tags["source_volume.id"] = SourceVolumeID
			}

			start := ""
			var limit int
			fmt.Printf("Please enter max number of snapshots entries per page to be returned(Optional): ")
			_, _ = fmt.Scanf("%d", &limit)
			for {
				Snapobj1, er11 := sess.ListSnapshots(limit, start, tags)
				if er11 == nil {
					ctxLogger.Info("Successfully got snapshot list================>", zap.Reflect("SnapshotList", *Snapobj1))
					if Snapobj1.Next != "" {
						fmt.Printf("\n\nFetching next set of snapshots starting from %v...\n\n", Snapobj1.Next)
						start = Snapobj1.Next
						continue
					}
				} else {
					er11 = updateRequestID(er11, requestID)
					ctxLogger.Info("failed to list snapshots================>", zap.Reflect("Error", er11))
				}
				break
			}
			fmt.Printf("\n\n")
		} else if choiceN == 4 {
			fmt.Println("You selected choice to get snapshot from volume by snapshot name")
			SourceVolumeID := ""
			SnapshotName := ""
			fmt.Printf("Please enter sourceVolumeID to filter snapshots: ")
			_, _ = fmt.Scanf("%s", &SourceVolumeID)

			fmt.Printf("Please enter SnapshotName to filter snapshots: ")
			_, _ = fmt.Scanf("%s", &SnapshotName)

			snapdetails, errr := sess.GetSnapshotByName(SnapshotName, SourceVolumeID)
			fmt.Printf("\n\n")
			if errr == nil {
				ctxLogger.Info("Successfully retrieved the snapshot details ================>", zap.Reflect("Snapshot ID", snapshotID))
				ctxLogger.Info("Snapshot details ================>", zap.Reflect("SnapshotDetails", snapdetails))
			} else {
				ctxLogger.Info("Failed to get snapshot details ================>", zap.Reflect("Snapshot ID", snapshotID), zap.Reflect("Error", errr))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 5 {
			fmt.Printf("Please enter sourceVolumeID of snapshot: ")
			_, _ = fmt.Scanf("%s", &SourceVolumeID)

			fmt.Printf("Please enter Snapshot ID: ")
			_, _ = fmt.Scanf("%s", &snapshotID)
			snapdetails, errr := sess.GetSnapshot(snapshotID, SourceVolumeID)
			fmt.Printf("\n\n")
			if errr == nil {
				ctxLogger.Info("Successfully retrieved the snapshot details ================>", zap.Reflect("Snapshot ID", snapshotID))
				ctxLogger.Info("Snapshot details ================>", zap.Reflect("SnapshotDetails", snapdetails))
			} else {
				ctxLogger.Info("Failed to get snapshot details ================>", zap.Reflect("Snapshot ID", snapshotID), zap.Reflect("Error", errr))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 6 {
			fmt.Println("You selected choice to order snapshot -- Not supported")
			fmt.Printf("\n\n")
		} else if choiceN == 7 {
			fmt.Println("You selected choice to Create volume from snapshot -- Not supported")
			fmt.Printf("\n\n")
		} else if choiceN == 8 {
			fmt.Println("You selected choice to delete volume")
			volume := &provider.Volume{}
			fmt.Printf("Please enter volume ID for delete:")
			_, _ = fmt.Scanf("%s", &volumeID)
			volume.VolumeID = volumeID
			er11 = sess.DeleteVolume(volume)
			if er11 == nil {
				ctxLogger.Info("SUCCESSFULLY deleted volume ================>", zap.Reflect("Volume ID", volumeID))
			} else {
				er11 = updateRequestID(er11, requestID)
				ctxLogger.Info("FAILED volume deletion================>", zap.Reflect("Volume ID", volumeID), zap.Reflect("Error", er11))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 9 {
			fmt.Println("You selected choice to delete snapshot")
			snapshot := &provider.Snapshot{}

			fmt.Printf("Please enter sourceVolumeID of snapshot: ")
			_, _ = fmt.Scanf("%s", &SourceVolumeID)
			fmt.Printf("Please enter snapshot ID for delete:")
			_, _ = fmt.Scanf("%s", &snapshotID)
			snapshot.SnapshotID = snapshotID
			snapshot.VolumeID = SourceVolumeID
			er11 = sess.DeleteSnapshot(snapshot)
			if er11 == nil {
				ctxLogger.Info("Successfully deleted snapshot ================>", zap.Reflect("Snapshot ID", snapshotID))
			} else {
				er11 = updateRequestID(er11, requestID)
				ctxLogger.Info("failed snapshot deletion================>", zap.Reflect("Snapshot ID", snapshotID), zap.Reflect("Error", er11))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 10 {
			fmt.Println("You selected choice to list all snapshot -- Not supported")
			fmt.Printf("\n\n")
		} else if choiceN == 11 {
			fmt.Println("Get volume ID by using order ID -- Not supported")
			fmt.Printf("\n\n")
		} else if choiceN == 12 {
			fmt.Println("Authorize volume -- Not supported")
			fmt.Printf("\n\n")
		} else if choiceN == 13 {
			fmt.Println("You selected choice to Create VPC volume")
			volume := &provider.Volume{}
			/* volume.VolumeEncryptionKey = &provider.VolumeEncryptionKey{
				CRN: "crn:v1:bluemix:public:kms:us-south:a/b661e758bf044d2d928fef7900d5130b:f5be30ff-b5d8-4edd-9f5a-cc313b3584db:key:8e282989-19e8-4415-b6e3-b9d2ec1911d0",
			} */

			volume.VPCVolume.ResourceGroup = &provider.ResourceGroup{}

			var (
				profile         string
				zone            string
				volumeName      string
				resourceGroup   string
				volSize         int
				iops, bandwidth int
			)

			fmt.Printf("\nPlease enter profile name (supported: dp2, rfs): ")
			_, _ = fmt.Scanf("%s", &profile)
			volume.VPCVolume.Profile = &provider.Profile{Name: profile}

			fmt.Printf("Enter zone: ")
			_, _ = fmt.Scanf("%s", &zone)
			volume.Az = zone

			// Always prompt for IOPS
			fmt.Printf("\nEnter IOPS (optional, 0 to skip): ")
			_, _ = fmt.Scanf("%d", &iops)
			iopsStr := fmt.Sprintf("%d", iops)
			volume.Iops = &iopsStr

			// Always prompt for Bandwidth
			fmt.Printf("\nEnter Bandwidth (optional, 0 to skip): ")
			_, _ = fmt.Scanf("%d", &bandwidth)
			volume.Bandwidth = int32(bandwidth)

			fmt.Printf("\nPlease enter volume name: ")
			_, _ = fmt.Scanf("%s", &volumeName)
			volume.Name = &volumeName

			fmt.Printf("\nPlease enter volume size (Specify 10 GB - 16 TB of capacity in 1 GB increments): ")
			_, _ = fmt.Scanf("%d", &volSize)
			volume.Capacity = &volSize

			fmt.Printf("\nPlease enter snapshotCRN: ")
			_, _ = fmt.Scanf("%s", &snapshotID)
			volume.SnapshotCRN = snapshotID

			fmt.Printf("\nPlease enter resource group ID:")
			_, _ = fmt.Scanf("%s", &resourceGroup)
			if volume.VPCVolume.ResourceGroup == nil {
				volume.VPCVolume.ResourceGroup = &provider.ResourceGroup{}
			}
			volume.VPCVolume.ResourceGroup.ID = resourceGroup

			//volume.SnapshotSpace = &volSize
			//volume.VPCVolume.Tags = []string{"Testing VPC Volume"}
			volumeObj, errr := sess.CreateVolume(*volume)
			if errr == nil {
				ctxLogger.Info("SUCCESSFULLY created volume...", zap.Reflect("volumeObj", volumeObj))
			} else {
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED to create volume...", zap.Reflect("StorageType", volume.ProviderType), zap.Reflect("Error", errr))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 14 {
			fmt.Println("You selected choice to order VPC snapshot -- Not Supported")
			fmt.Printf("\n\n")
		} else if choiceN == 15 {
			volumeAccessPointRequest := provider.VolumeAccessPointRequest{}
			var volumeTargetName string
			var vpcID string

			fmt.Println("You selected choice to create volume target")
			fmt.Printf("Please enter volume ID: ")
			_, _ = fmt.Scanf("%s", &volumeID)
			_, errr := sess.GetVolume(volumeID)
			if errr != nil {
				ctxLogger.Info("Provider error is ================>", zap.Reflect("ErrorType", userError.GetErrorType(errr)))
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED to get volume details ================>", zap.Reflect("VolumeID", volumeID), zap.Reflect("Error", errr))
				continue
			}
			volumeAccessPointRequest.VolumeID = volumeID

			fmt.Printf("\nPlease enter VPC ID: ")
			_, _ = fmt.Scanf("%s", &vpcID)
			volumeAccessPointRequest.VPCID = vpcID

			fmt.Printf("\nPlease enter volume target name: ")
			_, _ = fmt.Scanf("%s", &volumeTargetName)
			volumeAccessPointRequest.AccessPointName = volumeTargetName

			volumeAccessPointResponse, errr := sess.CreateVolumeAccessPoint(volumeAccessPointRequest)

			if errr == nil {
				ctxLogger.Info("SUCCESSFULLY created volume target...", zap.Reflect("volumetTargetObj", volumeAccessPointResponse))
			} else {
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED to create volume target...", zap.Reflect("Error", errr))
			}
			fmt.Printf("\n\n")

		} else if choiceN == 16 {
			var vpcID string
			volumeAccessPointRequest := provider.VolumeAccessPointRequest{}
			fmt.Println("You selected choice to delete volume target")
			fmt.Printf("Please enter volume ID: ")
			_, _ = fmt.Scanf("%s", &volumeID)
			_, errr := sess.GetVolume(volumeID)
			if errr != nil {
				ctxLogger.Info("Provider error is ================>", zap.Reflect("ErrorType", userError.GetErrorType(errr)))
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED to get volume details ================>", zap.Reflect("VolumeID", volumeID), zap.Reflect("Error", errr))
			}
			volumeAccessPointRequest.VolumeID = volumeID

			fmt.Printf("Please enter target ID: ")
			_, _ = fmt.Scanf("%s", &targetID)

			if targetID == "" {
				fmt.Printf("Please enter VPC ID: ")
				_, _ = fmt.Scanf("%s", &vpcID)
				volumeAccessPointRequest.VPCID = vpcID
			} else {
				volumeAccessPointRequest.AccessPointID = targetID
			}

			volumeAccessPointResponse, errr := sess.DeleteVolumeAccessPoint(volumeAccessPointRequest)

			if errr == nil {
				ctxLogger.Info("SUCCESSFULLY deleted volume target...", zap.Reflect("volumeAccessPointResponse", volumeAccessPointResponse))
			} else {
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED to delete volume target...", zap.Reflect("Error", errr))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 17 {
			fmt.Println("You selected get VPC volume by name")
			volumeName := ""
			fmt.Printf("Please enter volume Name to get the details: ")
			_, _ = fmt.Scanf("%s", &volumeName)
			volumeobj1, er11 := sess.GetVolumeByName(volumeName)
			if er11 == nil {
				ctxLogger.Info("Successfully got VPC volume details ================>", zap.Reflect("VolumeDetail", volumeobj1))
			} else {
				er11 = updateRequestID(er11, requestID)
				ctxLogger.Info("failed to order snapshot space================>", zap.Reflect("VolumeName", volumeName), zap.Reflect("Error", er11))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 18 {
			fmt.Println("You selected list volumes")
			tags := map[string]string{}
			volName := ""
			resourceGroupID := ""

			fmt.Printf("Please enter volume Name to filter volumes(Optional): ")
			_, _ = fmt.Scanf("%s", &volName)
			if volName != "" {
				tags["name"] = volName
			}

			fmt.Printf("\nPlease enter resource group ID to filter volumes(Optional): ")
			_, _ = fmt.Scanf("%s", &resourceGroupID)
			if resourceGroupID != "" {
				tags["resource_group.id"] = resourceGroupID
			}

			start := ""
			var limit int
			fmt.Printf("Please enter max number of volume entries per page to be returned(Optional): ")
			_, _ = fmt.Scanf("%d", &limit)
			for {
				volumeobj1, er11 := sess.ListVolumes(limit, start, tags)
				if er11 == nil {
					ctxLogger.Info("Successfully got volumes list================>", zap.Reflect("VolumesList", *volumeobj1))
					if volumeobj1.Next != "" {
						fmt.Printf("\n\nFetching next set of volumes starting from %v...\n\n", volumeobj1.Next)
						start = volumeobj1.Next
						continue
					}
				} else {
					er11 = updateRequestID(er11, requestID)
					ctxLogger.Info("failed to list volumes================>", zap.Reflect("Error", er11))
				}
				break
			}
			fmt.Printf("\n\n")
		} else if choiceN == 19 {
			var vpcID string
			volumeAccessPointRequest := provider.VolumeAccessPointRequest{}

			fmt.Println("You selected choice to get volume target")
			fmt.Printf("Please enter volume ID: ")
			_, _ = fmt.Scanf("%s", &volumeID)
			_, errr := sess.GetVolume(volumeID)
			if errr != nil {
				ctxLogger.Info("Provider error is ================>", zap.Reflect("ErrorType", userError.GetErrorType(errr)))
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED to get volume details ================>", zap.Reflect("VolumeID", volumeID), zap.Reflect("Error", errr))
			}
			volumeAccessPointRequest.VolumeID = volumeID

			fmt.Printf("Please enter target ID: ")
			_, _ = fmt.Scanf("%s", &targetID)

			if targetID == "" {
				fmt.Printf("Please enter VPC ID: ")
				_, _ = fmt.Scanf("%s", &vpcID)
				volumeAccessPointRequest.VPCID = vpcID
			} else {
				volumeAccessPointRequest.AccessPointID = targetID
			}

			volumeAccessPointResponse, errr := sess.GetVolumeAccessPoint(volumeAccessPointRequest)

			if errr == nil {
				ctxLogger.Info("SUCCESSFULLY fetched volume target...", zap.Reflect("volumeAccessPointResponse", volumeAccessPointResponse))
			} else {
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED to fetched volume target...", zap.Reflect("Error", errr))
			}
			fmt.Printf("\n\n")

		} else if choiceN == 20 {
			volumeAccessPointRequest := provider.VolumeAccessPointRequest{}

			var vpcID string

			fmt.Println("You selected choice to wait for create volume target")
			fmt.Printf("Please enter volume ID: ")
			_, _ = fmt.Scanf("%s", &volumeID)
			_, errr := sess.GetVolume(volumeID)
			if errr != nil {
				ctxLogger.Info("Provider error is ================>", zap.Reflect("ErrorType", userError.GetErrorType(errr)))
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED to get volume details ================>", zap.Reflect("VolumeID", volumeID), zap.Reflect("Error", errr))
			}
			volumeAccessPointRequest.VolumeID = volumeID

			fmt.Printf("Please enter target ID: ")
			_, _ = fmt.Scanf("%s", &targetID)

			if targetID == "" {
				fmt.Printf("Please enter VPC ID: ")
				_, _ = fmt.Scanf("%s", &vpcID)
				volumeAccessPointRequest.VPCID = vpcID
			} else {
				volumeAccessPointRequest.AccessPointID = targetID
			}

			volumeAccessPointResponse, errr := sess.WaitForCreateVolumeAccessPoint(volumeAccessPointRequest)

			if errr == nil {
				ctxLogger.Info("SUCCESSFULLY volume target is stable for mount...", zap.Reflect("volumeAccessPointResponse", volumeAccessPointResponse))
			} else {
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED volume target is not stable for mount...", zap.Reflect("Error", errr))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 21 {

			var vpcID string
			volumeAccessPointRequest := provider.VolumeAccessPointRequest{}

			fmt.Println("You selected choice to wait for delete volume target")
			fmt.Printf("Please enter volume ID: ")
			_, _ = fmt.Scanf("%s", &volumeID)
			_, errr := sess.GetVolume(volumeID)
			if errr != nil {
				ctxLogger.Info("Provider error is ================>", zap.Reflect("ErrorType", userError.GetErrorType(errr)))
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED to get volume details ================>", zap.Reflect("VolumeID", volumeID), zap.Reflect("Error", errr))
			}
			volumeAccessPointRequest.VolumeID = volumeID

			fmt.Printf("Please enter target ID: ")
			_, _ = fmt.Scanf("%s", &targetID)

			if targetID == "" {
				fmt.Printf("Please enter VPC ID: ")
				_, _ = fmt.Scanf("%s", &vpcID)
				volumeAccessPointRequest.VPCID = vpcID
			} else {
				volumeAccessPointRequest.AccessPointID = targetID
			}

			errr = sess.WaitForDeleteVolumeAccessPoint(volumeAccessPointRequest)

			if errr == nil {
				ctxLogger.Info("SUCCESSFULLY wait for delete volume target is successfull...", zap.Reflect("volumeAccessPointRequest", volumeAccessPointRequest))
			} else {
				errr = updateRequestID(errr, requestID)
				ctxLogger.Info("FAILED wait for delete volume target...", zap.Reflect("Error", errr))
			}
			fmt.Printf("\n\n")
		} else if choiceN == 22 {
			var capacity int64
			fmt.Println("You selected choice to expand volume")
			share := &provider.ExpandVolumeRequest{}
			fmt.Printf("Please enter volume ID to exand: ")
			_, _ = fmt.Scanf("%s", &volumeID)
			fmt.Printf("Please enter new capacity: ")
			_, _ = fmt.Scanf("%d", &capacity)
			share.VolumeID = volumeID
			share.Capacity = capacity
			// Call ExpandVolume
			expandedVolumeSize, er11 := sess.ExpandVolume(*share)
			if er11 == nil {
				ctxLogger.Info("Successfully expanded volume ================>", zap.Reflect("Volume ID", expandedVolumeSize))
			} else {
				er11 = updateRequestID(er11, requestID)
				ctxLogger.Info("Failed to expand =================>", zap.Reflect("Volume ID", volumeID), zap.Reflect("Error", er11))
			}
			fmt.Printf("\n\n")

		} else if choiceN == 23 {
			fmt.Println("You selected get share profile")
			profileName := ""
			fmt.Printf("Please enter profile name to get the details: ")
			_, _ = fmt.Scanf("%s", &profileName)
			profile, er11 := sess.GetVolumeProfileByName(profileName)
			if er11 == nil {
				ctxLogger.Info("Successfully got Volume Profile", zap.Reflect("profile", profile))
			} else {
				er11 = updateRequestID(er11, requestID)
				ctxLogger.Info("failed to get Profile name================>", zap.Reflect("profileName", profileName), zap.Reflect("Error", er11))
			}
			fmt.Printf("\n\n")
		} else {
			fmt.Println("No right choice")
			return
		}
	}
}
