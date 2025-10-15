/**
 * Copyright 2024 IBM Corp.
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
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/IBM/ibmcloud-volume-file-vpc/e2e/testsuites"
	. "github.com/onsi/ginkgo/v2"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clientset "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/kubernetes/test/e2e/framework"
	admissionapi "k8s.io/pod-security-admission/api"
)

const (
	defaultSecret              = ""
	waitForPackageInstallation = 2 * time.Minute
)

var (
	testResultFile = os.Getenv("E2E_TEST_RESULT")
	err            error
	fpointer       *os.File
	sc             = os.Getenv("SC")
	sc_retain      = os.Getenv("SC_RETAIN")
)

func createCustomRfsSC(cs clientset.Interface, name string, params map[string]string) (*storagev1.StorageClass, error) {
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner: "vpc.file.csi.ibm.io",
		Parameters:  params,
	}
	reclaimPolicy := v1.PersistentVolumeReclaimDelete
	volumeBindingMode := storagev1.VolumeBindingImmediate
	sc.ReclaimPolicy = &reclaimPolicy
	sc.VolumeBindingMode = &volumeBindingMode

	return cs.StorageV1().StorageClasses().Create(context.TODO(), sc, metav1.CreateOptions{})
}

// Delete a custom RFS StorageClass
func deleteCustomRfsSC(cs clientset.Interface, name string) error {
	err := cs.StorageV1().StorageClasses().Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Printf("StorageClass %s already deleted\n", name)
			return nil
		}
		return fmt.Errorf("failed to delete StorageClass %q: %w", name, err)
	}
	fmt.Printf("StorageClass %s deleted successfully\n", name)
	return nil
}

func CreateRFSPVC(pvcName, sc, namespace string, throughput int, rfsPvcSize string, cs kubernetes.Interface) {
	// Delete old PVC if exists
	_ = cs.CoreV1().PersistentVolumeClaims(namespace).Delete(context.Background(), pvcName, metav1.DeleteOptions{})

	customSCName := sc

	// Create PVC object
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &customSCName,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(rfsPvcSize),
				},
			},
		},
	}

	// Create the PVC
	_, err := cs.CoreV1().PersistentVolumeClaims(namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		panic(fmt.Sprintf("Failed to create PVC: %v", err))
	}

	pollInterval := 5 * time.Second
	pollTimeout := 5 * time.Minute
	// Wait for PVC status
	err = wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		updatedPVC, err := cs.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return updatedPVC.Status.Phase == corev1.ClaimBound, nil
	})
}

var _ = BeforeSuite(func() {
	log.Print("Successfully construct the service client instance")
	testsuites.InitializeVPCClient()
})

var _ = Describe("[ics-e2e] [sc] [with-deploy] [retain] Dynamic Provisioning using retain SC with Deployment", func() {
	f := framework.NewDefaultFramework("ics-e2e-deploy")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		secretKey string
	)

	secretKey = os.Getenv("E2E_SECRET_ENCRYPTION_KEY")
	if secretKey == "" {
		secretKey = defaultSecret
	}

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
	})

	It("with retain sc: should create a pvc &pv, deployment resources, write and read to volume, delete the pod, write and read to volume again", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			panic(labelerr)
		}
		reclaimPolicy := v1.PersistentVolumeReclaimRetain
		fpointer, _ = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer fpointer.Close()

		var replicaCount = int32(1)
		pod := testsuites.PodDetails{
			Cmd:      "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 2; done",
			CmdExits: false,
			Volumes: []testsuites.VolumeDetails{
				{
					PVCName:       "ics-vol-dp2-",
					VolumeType:    sc_retain,
					FSType:        "ext4",
					ClaimSize:     "15Gi",
					ReclaimPolicy: &reclaimPolicy,
					MountOptions:  []string{"rw"},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisioneDeployWithVolWRTest{
			Pod: pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
				ExpectedString02: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
			ReplicaCount: replicaCount,
		}
		test.Run(cs, ns)
		if _, err = fpointer.WriteString(fmt.Sprintf("VPC-FILE-CSI-TEST: VERIFYING PVC CREATE/DELETE WITH %s STORAGE CLASS : PASS\n", sc_retain)); err != nil {
			panic(err)
		}
	})
})

var _ = Describe("[ics-e2e] [sc_rfs] Dynamic Provisioning for RFS SC with Deployment", func() {
	f := framework.NewDefaultFramework("ics-e2e-deploy")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs clientset.Interface
		ns *v1.Namespace
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
	})

	It("with rfs profile sc: should create a pvc, deployment resources, write and read to volume, delete the pod", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to patch namespace %s: %v\n", ns.Name, labelerr)
		}
		sc := "ibmc-vpc-file-regional"
		reclaimPolicy := v1.PersistentVolumeReclaimDelete

		var replicaCount = int32(1)
		pod := testsuites.PodDetails{
			Cmd:      "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 2; done",
			CmdExits: false,
			Volumes: []testsuites.VolumeDetails{
				{
					PVCName:       "ics-vol-rfs-",
					VolumeType:    sc,
					FSType:        "ext4",
					ClaimSize:     "15Gi",
					ReclaimPolicy: &reclaimPolicy,
					MountOptions:  []string{"rw"},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}

		test := testsuites.DynamicallyProvisioneDeployWithVolWRTest{
			Pod: pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
				ExpectedString02: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
			ReplicaCount: replicaCount,
		}
		test.Run(cs, ns)

		fpointer, _ = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		defer fpointer.Close()
		_, _ = fpointer.WriteString(fmt.Sprintf("VPC-FILE-CSI-TEST: VERIFYING PVC CREATE/DELETE WITH DEFAULT BANDWIDTH FOR %s STORAGE CLASS : PASS\n", sc))
	})

	It("with rfs profile sc : should create a pvc, deployment resources, write and read to volume, delete the pod with max bandwidth ", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to patch namespace %s: %v\n", ns.Name, labelerr)
		}
		sc := "ibmc-vpc-file-regional-max-bandwidth"
		reclaimPolicy := v1.PersistentVolumeReclaimDelete

		var replicaCount = int32(1)
		pod := testsuites.PodDetails{
			Cmd:      "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 2; done",
			CmdExits: false,
			Volumes: []testsuites.VolumeDetails{
				{
					PVCName:       "ics-vol-rfs-",
					VolumeType:    sc,
					FSType:        "ext4",
					ClaimSize:     "15Gi",
					ReclaimPolicy: &reclaimPolicy,
					MountOptions:  []string{"rw"},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}

		test := testsuites.DynamicallyProvisioneDeployWithVolWRTest{
			Pod: pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
				ExpectedString02: "hello world\nhello world\n",
			},
			ReplicaCount: replicaCount,
		}
		test.Run(cs, ns)

		fpointer, _ = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		defer fpointer.Close()
		_, _ = fpointer.WriteString(fmt.Sprintf("VPC-FILE-CSI-TEST: VERIFYING PVC CREATE/DELETE WITH MAX BANDWIDTH FOR %s STORAGE CLASS : PASS\n", sc))
	})

	It("with rfs profile sc: should provide default throughput and should create a pvc, deployment resources, write and read to volume, delete the pod", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to patch namespace %s: %v\n", ns.Name, labelerr)
		}
		params := map[string]string{
			"profile":    "rfs",
			"throughput": "0",
		}
		sc := "custom-rfs-sc"
		createCustomRfsSC(cs, "custom-rfs-sc", params)
		defer deleteCustomRfsSC(cs, sc)
		reclaimPolicy := v1.PersistentVolumeReclaimDelete

		var replicaCount = int32(1)
		pod := testsuites.PodDetails{
			Cmd:      "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 2; done",
			CmdExits: false,
			Volumes: []testsuites.VolumeDetails{
				{
					PVCName:       "ics-vol-rfs-",
					VolumeType:    sc,
					FSType:        "ext4",
					ClaimSize:     "15Gi",
					ReclaimPolicy: &reclaimPolicy,
					MountOptions:  []string{"rw"},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}

		test := testsuites.DynamicallyProvisioneDeployWithVolWRTest{
			Pod: pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
				ExpectedString02: "hello world\nhello world\n",
			},
			ReplicaCount: replicaCount,
		}
		test.Run(cs, ns)

		fpointer, _ = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		defer fpointer.Close()
		_, _ = fpointer.WriteString(fmt.Sprintf("VPC-FILE-CSI-TEST: VERIFYING PVC CREATE/DELETE WITH ZERO BANDWIDTH FOR %s STORAGE CLASS : PASS\n", sc))
	})

	It("with rfs profile sc: should fail when bandwidth is set to an invalid high value (9000)", func() {
		params := map[string]string{
			"profile":    "rfs",
			"throughput": "9000",
		}
		sc := "custom-rfs-sc-1"
		createCustomRfsSC(cs, sc, params)
		defer deleteCustomRfsSC(cs, sc)

		// Defer the deletion of the StorageClass object.
		defer func() {
			if err := cs.StorageV1().StorageClasses().Delete(context.Background(), "custom-rfs-sc-1", metav1.DeleteOptions{}); err != nil {
				fmt.Printf("Warning: failed to delete StorageClass custom-rfs-sc-1: %v\n", err)
			}
		}()

		// create pvc
		CreateRFSPVC("rfs-test-pvc", "rfs-test-sc", ns.Name, 9000, "10Gi", cs)

		// Defer the deletion of the PVC object.
		defer func() {
			if err := cs.CoreV1().PersistentVolumeClaims(ns.Name).Delete(context.Background(), "rfs-test-pvc", metav1.DeleteOptions{}); err != nil {
				fmt.Printf("Warning: failed to delete PVC rfs-test-pvc: %v\n", err)
			}
		}()

		fpointer, _ = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		defer fpointer.Close()

		_, _ = fpointer.WriteString(fmt.Sprintf("VPC-FILE-CSI-TEST: VERIFYING PVC CREATE FAIL WITH INVALID BANDWIDTH (9000) FOR %s STORAGE CLASS : PASS\n", sc))
	})

	It("with rfs profile sc: should fail when iops is provided for rfs profile", func() {
		params := map[string]string{
			"profile":    "rfs",
			"throughput": "100",
			"iops":       "36000",
		}
		sc := "custom-rfs-sc-2"
		createCustomRfsSC(cs, sc, params)
		defer deleteCustomRfsSC(cs, sc)
		// Defer the deletion of the StorageClass object.
		defer func() {
			if err := cs.StorageV1().StorageClasses().Delete(context.Background(), "custom-rfs-sc-2", metav1.DeleteOptions{}); err != nil {
				fmt.Printf("Warning: failed to delete StorageClass custom-rfs-sc-1: %v\n", err)
			}
		}()
		// create pvc
		CreateRFSPVC("rfs-test-pvc", "rfs-test-sc", ns.Name, 100, "10Gi", cs)
		// Defer the deletion of the PVC object.
		defer func() {
			if err := cs.CoreV1().PersistentVolumeClaims(ns.Name).Delete(context.Background(), "rfs-test-pvc", metav1.DeleteOptions{}); err != nil {
				fmt.Printf("Warning: failed to delete PVC rfs-test-pvc: %v\n", err)
			}
		}()

		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		defer fpointer.Close()

		_, _ = fpointer.WriteString(fmt.Sprintf("VPC-FILE-CSI-TEST: VERIFYING PVC CREATE FAIL WITH IOPS PARAM FOR %s STORAGE CLASS : PASS\n", sc))
	})

	It("with rfs profile sc: should fail when zone is provided for rfs profile", func() {
		params := map[string]string{
			"profile":    "rfs",
			"throughput": "100",
			"zone":       "us-south-1",
		}
		sc := "custom-rfs-sc-3"
		createCustomRfsSC(cs, sc, params)
		defer deleteCustomRfsSC(cs, sc)
		// Defer the deletion of the StorageClass object.
		defer func() {
			if err := cs.StorageV1().StorageClasses().Delete(context.Background(), "custom-rfs-sc-3", metav1.DeleteOptions{}); err != nil {
				fmt.Printf("Warning: failed to delete StorageClass custom-rfs-sc-1: %v\n", err)
			}
		}()
		// create pvc
		CreateRFSPVC("rfs-test-pvc", "rfs-test-sc", ns.Name, 100, "10Gi", cs)
		// Defer the deletion of the PVC object.
		defer func() {
			if err := cs.CoreV1().PersistentVolumeClaims(ns.Name).Delete(context.Background(), "rfs-test-pvc", metav1.DeleteOptions{}); err != nil {
				fmt.Printf("Warning: failed to delete PVC rfs-test-pvc: %v\n", err)
			}
		}()

		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		defer fpointer.Close()
		_, _ = fpointer.WriteString(fmt.Sprintf("VPC-FILE-CSI-TEST: VERIFYING PVC CREATE FAIL WITH ZONE PARAM FOR %s STORAGE CLASS : PASS\n", sc))
	})
})

var _ = Describe("[ics-e2e] [sc] [with-deploy] Dynamic Provisioning for dp2 SC with Deployment", func() {
	f := framework.NewDefaultFramework("ics-e2e-deploy")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		secretKey string
	)

	secretKey = os.Getenv("E2E_SECRET_ENCRYPTION_KEY")
	if secretKey == "" {
		secretKey = defaultSecret
	}

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
	})

	It("with dp2 sc: should create a pvc &pv, deployment resources, write and read to volume, delete the pod, write and read to volume again", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			panic(labelerr)
		}

		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer fpointer.Close()

		var replicaCount = int32(1)
		pod := testsuites.PodDetails{
			Cmd:      "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 2; done",
			CmdExits: false,
			Volumes: []testsuites.VolumeDetails{
				{
					PVCName:       "ics-vol-dp2-",
					VolumeType:    sc,
					FSType:        "ext4",
					ClaimSize:     "15Gi",
					ReclaimPolicy: &reclaimPolicy,
					MountOptions:  []string{"rw"},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisioneDeployWithVolWRTest{
			Pod: pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
				ExpectedString02: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
			ReplicaCount: replicaCount,
		}
		test.Run(cs, ns)
		if _, err = fpointer.WriteString(fmt.Sprintf("VPC-FILE-CSI-TEST: VERIFYING PVC CREATE/DELETE WITH %s STORAGE CLASS : PASS\n", sc)); err != nil {
			panic(err)
		}
	})
})

var _ = Describe("[ics-e2e] [sc] [same-node] [with-deploy] Dynamic Provisioning for dp2 SC with Deployment running multiple pods on same node", func() {
	f := framework.NewDefaultFramework("ics-e2e-deploy")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		secretKey string
	)

	secretKey = os.Getenv("E2E_SECRET_ENCRYPTION_KEY")
	if secretKey == "" {
		secretKey = defaultSecret
	}

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
	})

	It("with dp2 sc: should create a pvc &pv, deployment resources, write and read to volume, delete the pod, write and read to volume again", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			panic(labelerr)
		}
		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer fpointer.Close()

		var replicaCount = int32(4)
		pod := testsuites.PodDetails{
			Cmd:      "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 2; done",
			CmdExits: false,
			Volumes: []testsuites.VolumeDetails{
				{
					PVCName:       "ics-vol-dp2-",
					VolumeType:    sc,
					FSType:        "ext4",
					ClaimSize:     "15Gi",
					ReclaimPolicy: &reclaimPolicy,
					MountOptions:  []string{"rw"},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisioneDeployWithVolWRTest{
			Pod: pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
				ExpectedString02: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
			ReplicaCount: replicaCount,
		}
		test.Run(cs, ns)
		if _, err = fpointer.WriteString("VPC-FILE-CSI-TEST: VERIFYING MULTI-POD READ/WRITE ON SAME NODE BY USING DEPLOYMENT: PASS\n"); err != nil {
			panic(err)
		}
	})
})

var _ = Describe("[ics-e2e] [sc] [rwo] [with-deploy] Dynamic Provisioning for dp2 SC in RWO Mode with Deployment", func() {
	f := framework.NewDefaultFramework("ics-e2e-deploy")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs           clientset.Interface
		ns           *v1.Namespace
		cleanupFuncs []func()
		volList      []testsuites.VolumeDetails
		cmdLongLife  string
		maxPVC       int
		maxPOD       int
		secretKey    string
	)

	secretKey = os.Getenv("E2E_SECRET_ENCRYPTION_KEY")
	if secretKey == "" {
		secretKey = defaultSecret
	}

	maxPOD = 4
	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		cleanupFuncs = make([]func(), 0)

		//cmdShotLife = "df -h; echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data"
		cmdLongLife = "df -h; echo 'hello world' > /mnt/test-1/data && while true; do sleep 2; done"

		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		accessMode := v1.ReadWriteOnce

		volList = []testsuites.VolumeDetails{
			{
				PVCName:       "ics-vol-dp2-",
				VolumeType:    sc,
				AccessMode:    &accessMode,
				ClaimSize:     "15Gi",
				ReclaimPolicy: &reclaimPolicy,
				MountOptions:  []string{"rw"},
				VolumeMount: testsuites.VolumeMountDetails{
					NameGenerate:      "test-volume-",
					MountPathGenerate: "/mnt/test-",
				},
			},
		}
	})

	It("with dp2 sc: should create a pvc &pv with RWO mode , deployment resources, write and read to volume, delete the pod, write and read to volume again", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			panic(labelerr)
		}

		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer fpointer.Close()

		var execCmd string
		var cmdExits bool
		var vols []testsuites.VolumeDetails
		var pods []testsuites.PodDetails

		maxPVC = 1
		vollistLen := len(volList)
		fmt.Println("vollistLen", vollistLen)
		vols = make([]testsuites.VolumeDetails, 0)
		xi := 0
		for i := 0; vollistLen > 0 && i < maxPVC; i++ {
			if xi >= vollistLen {
				xi = 0
			}
			vol := volList[xi]
			vols = append(vols, vol)
			xi = xi + 1
		}

		// Create PVC
		execCmd = cmdLongLife
		cmdExits = false
		for n := range vols {
			_, funcs := vols[n].SetupDynamicPersistentVolumeClaim(cs, ns, false)
			cleanupFuncs = append(cleanupFuncs, funcs...)
		}

		for i := range cleanupFuncs {
			defer cleanupFuncs[i]()
		}

		pods = make([]testsuites.PodDetails, 0)
		for i := 0; i < maxPOD; i++ {
			pod := testsuites.PodDetails{
				Cmd:      execCmd,
				CmdExits: cmdExits,
				Volumes:  vols,
			}
			pods = append(pods, pod)
		}
		test := testsuites.DynamicallyProvisioneMultiPodWithVolTest{
			Pods:     pods,
			PodCheck: nil,
		}

		test.RunAsync(cs, ns)
		if _, err = fpointer.WriteString("VPC-FILE-CSI-TEST: VERIFYING PVC WITH RWO MODE : PASS\n"); err != nil {
			panic(err)
		}
	})
})

var _ = Describe("[ics-e2e] [sc] [with-daemonset] Dynamic Provisioning using daemonsets", func() {
	f := framework.NewDefaultFramework("ics-e2e-daemonsets")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs clientset.Interface
		ns *v1.Namespace
	)
	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
	})
	It("With 5iops sc: should creat daemonset resources, write and read to volume", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			panic(labelerr)
		}
		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer fpointer.Close()

		headlessService := testsuites.NewHeadlessService(cs, "ics-e2e-service-", ns.Name, "test")
		service := headlessService.Create()
		defer headlessService.Cleanup()

		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' > /mnt/test-1/data && while true; do sleep 2; done",
			Volumes: []testsuites.VolumeDetails{
				{
					PVCName:       "ics-vol-dp2-",
					VolumeType:    sc,
					FSType:        "ext4",
					ClaimSize:     "20Gi",
					ReclaimPolicy: &reclaimPolicy,
					MountOptions:  []string{"rw"},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}

		test := testsuites.DaemonsetWithVolWRTest{
			Pod: pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
			},
			Labels:      service.Labels,
			ServiceName: service.Name,
		}
		test.Run(cs, ns, false)
		if _, err = fpointer.WriteString("VPC-FILE-CSI-TEST: VERIFYING MULTI-ZONE/MULTI-NODE READ/WRITE BY USING DAEMONSET : PASS\n"); err != nil {
			panic(err)
		}
	})
})

var _ = Describe("[ics-e2e] [resize] [pv] Dynamic Provisioning and resize pv", func() {
	f := framework.NewDefaultFramework("ics-e2e-pods")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		secretKey string
	)

	secretKey = os.Getenv("E2E_SECRET_ENCRYPTION_KEY")
	if secretKey == "" {
		secretKey = defaultSecret
	}

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
	})

	It("with dp2 sc: should create a pvc & pv, pod resources, and resize the volume", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			panic(labelerr)
		}
		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer fpointer.Close()
		pods := []testsuites.PodDetails{
			{
				Cmd:      "echo 'hello world' > /mnt/test-1/data && while true; do sleep 2; done",
				CmdExits: false,
				Volumes: []testsuites.VolumeDetails{
					{
						PVCName:       "ics-vol-dp2-",
						VolumeType:    sc,
						ClaimSize:     "20Gi",
						ReclaimPolicy: &reclaimPolicy,
						MountOptions:  []string{"rw"},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedResizeVolumeTest{
			Pods: pods,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
				ExpectedString02: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
			// ExpandVolSize is in Gi i.e, 40Gi
			ExpandVolSizeG: 40,
			ExpandedSize:   40,
		}
		test.Run(cs, ns)
		if _, err = fpointer.WriteString("VPC-FILE-CSI-TEST: VERIFYING PVC EXPANSION BY USING DEPLOYMENT: PASS\n"); err != nil {
			panic(err)
		}
	})
})

// **EIT test cases**
var _ = Describe("[ics-e2e] [eit] Dynamic Provisioning for ibmc-vpc-file-eit SC with DaemonSet", func() {
	f := framework.NewDefaultFramework("ics-e2e-daemonsets")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs clientset.Interface
		ns *v1.Namespace
	)
	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		// Patch 'addon-vpc-file-csi-driver-configmap' to enable eit from operator
		secondary_wp := os.Getenv("cluster_worker_pool")
		fmt.Printf("cluster_worker_pool: %s", secondary_wp)
		wp_list := "default"
		if secondary_wp != "" {
			wp_list = wp_list + "," + secondary_wp
		}
		cmData := map[string]interface{}{
			"data": map[string]string{
				"ENABLE_EIT":               "true",
				"EIT_ENABLED_WORKER_POOLS": wp_list,
			},
		}
		cmDataBytes, err := json.Marshal(cmData)
		if err != nil {
			panic(err)
		}

		var cm *v1.ConfigMap
		cm, err = cs.CoreV1().ConfigMaps("kube-system").Patch(context.TODO(), "addon-vpc-file-csi-driver-configmap", types.MergePatchType, cmDataBytes, metav1.PatchOptions{})
		if err != nil {
			panic(err)
		}

		fmt.Println("Updated ConfigMap 'addon-vpc-file-csi-driver-configmap': ", cm.Data)

		// Add wait for packages to be installed on the system
		fmt.Printf("Sleep for %s to install EIT packages...", waitForPackageInstallation)
		time.Sleep(waitForPackageInstallation)
		cm_status, err := cs.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "file-csi-driver-status", metav1.GetOptions{})
		if err != nil {
			panic(err)
		}
		eitEnabledWorkerNodes, exists := cm_status.Data["EIT_ENABLED_WORKER_NODES"]
		if !exists {
			fmt.Println("EIT_ENABLED_WORKER_NODES not found in ConfigMap")
			err = fmt.Errorf("unknown problem with 'file-csi-driver-status' configmap")
			panic(err)
		}
		// Print the content of EIT_ENABLED_WORKER_NODES
		fmt.Println("EIT_ENABLED_WORKER_NODES:")
		fmt.Println(eitEnabledWorkerNodes)
	})
	It("With eit SC: should create pv, pvc, daemonSet resources. Write and read to volume.", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			panic(labelerr)
		}
		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer fpointer.Close()

		headlessService := testsuites.NewHeadlessService(cs, "ics-e2e-service-", ns.Name, "test")
		service := headlessService.Create()
		defer headlessService.Cleanup()

		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' > /mnt/test-1/data && while true; do sleep 2; done",
			Volumes: []testsuites.VolumeDetails{
				{
					PVCName:       "ics-vol-dp2-",
					VolumeType:    "ibmc-vpc-file-eit",
					FSType:        "ibmshare",
					ClaimSize:     "10Gi",
					ReclaimPolicy: &reclaimPolicy,
					MountOptions:  []string{"rw"},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}

		test := testsuites.DaemonsetWithVolWRTest{
			Pod: pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
			},
			Labels:      service.Labels,
			ServiceName: service.Name,
		}
		test.Run(cs, ns, false)
		if _, err = fpointer.WriteString("VPC-FILE-CSI-TEST-EIT: VERIFYING MULTI-ZONE/MULTI-NODE READ/WRITE BY USING DAEMONSET : PASS\n"); err != nil {
			panic(err)
		}
	})
	AfterEach(func() {
		cmData := map[string]interface{}{
			"data": map[string]string{
				"ENABLE_EIT": "false",
			},
		}
		cmDataBytes, err := json.Marshal(cmData)
		if err != nil {
			panic(err)
		}

		_, err = cs.CoreV1().ConfigMaps("kube-system").Patch(context.TODO(), "addon-vpc-file-csi-driver-configmap", types.MergePatchType, cmDataBytes, metav1.PatchOptions{})
		if err != nil {
			panic(err)
		}

		// Add wait for packages to be uninstalled from the system
		fmt.Printf("Sleep for %s to uninstall EIT packages...", waitForPackageInstallation)
		time.Sleep(waitForPackageInstallation)
	})
})

var _ = Describe("[ics-e2e] [eit] Dynamic Provisioning OF EIT VOLUME AND RESIZE PVC USING POD", func() {
	f := framework.NewDefaultFramework("ics-e2e-pods")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		secretKey string
	)

	secretKey = os.Getenv("E2E_SECRET_ENCRYPTION_KEY")
	if secretKey == "" {
		secretKey = defaultSecret
	}

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		// Patch 'addon-vpc-file-csi-driver-configmap' to enable eit from operator
		secondary_wp := os.Getenv("cluster_worker_pool")
		wp_list := "default"
		if secondary_wp != "" {
			wp_list = wp_list + "," + secondary_wp
		}
		cmData := map[string]interface{}{
			"data": map[string]string{
				"ENABLE_EIT":               "true",
				"EIT_ENABLED_WORKER_POOLS": wp_list,
			},
		}
		cmDataBytes, err := json.Marshal(cmData)
		if err != nil {
			panic(err)
		}

		var cm *v1.ConfigMap
		cm, err = cs.CoreV1().ConfigMaps("kube-system").Patch(context.TODO(), "addon-vpc-file-csi-driver-configmap", types.MergePatchType, cmDataBytes, metav1.PatchOptions{})
		if err != nil {
			panic(err)
		}

		fmt.Println("Updated ConfigMap 'addon-vpc-file-csi-driver-configmap': ", cm.Data)

		// Add wait for packages to be installed on the system
		fmt.Printf("Sleep for %s to install EIT packages...", waitForPackageInstallation)
		time.Sleep(waitForPackageInstallation)
		cm_status, err := cs.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "file-csi-driver-status", metav1.GetOptions{})
		if err != nil {
			panic(err)
		}
		eitEnabledWorkerNodes, exists := cm_status.Data["EIT_ENABLED_WORKER_NODES"]
		if !exists {
			fmt.Println("EIT_ENABLED_WORKER_NODES not found in ConfigMap")
			err = fmt.Errorf("unknown problem with 'file-csi-driver-status' configmap")
			panic(err)
		}
		// Print the content of EIT_ENABLED_WORKER_NODES
		fmt.Println("EIT_ENABLED_WORKER_NODES:")
		fmt.Println(eitEnabledWorkerNodes)
	})

	It("should create pv, pvc and pod resources, and resize the volume", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			panic(labelerr)
		}
		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer fpointer.Close()
		pods := []testsuites.PodDetails{
			{
				Cmd:      "echo 'hello world' > /mnt/test-1/data && while true; do sleep 2; done",
				CmdExits: false,
				Volumes: []testsuites.VolumeDetails{
					{
						PVCName:       "ics-vol-dp2-",
						VolumeType:    "ibmc-vpc-file-eit",
						ClaimSize:     "10Gi",
						ReclaimPolicy: &reclaimPolicy,
						MountOptions:  []string{"rw"},
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedResizeVolumeTest{
			Pods: pods,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
				ExpectedString02: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
			// ExpandVolSize is in Gi i.e, 40Gi
			ExpandVolSizeG: 40,
			ExpandedSize:   40,
		}
		test.Run(cs, ns)
		if _, err = fpointer.WriteString("VPC-FILE-CSI-TEST-EIT: VERIFYING PVC EXPANSION USING POD: PASS\n"); err != nil {
			panic(err)
		}
	})
	AfterEach(func() {
		cmData := map[string]interface{}{
			"data": map[string]string{
				"ENABLE_EIT": "false",
			},
		}
		cmDataBytes, err := json.Marshal(cmData)
		if err != nil {
			panic(err)
		}

		_, err = cs.CoreV1().ConfigMaps("kube-system").Patch(context.TODO(), "addon-vpc-file-csi-driver-configmap", types.MergePatchType, cmDataBytes, metav1.PatchOptions{})
		if err != nil {
			panic(err)
		}

		// Add wait for packages to be uninstalled from the system
		fmt.Printf("Sleep for %s to uninstall EIT packages...", waitForPackageInstallation)
		time.Sleep(waitForPackageInstallation)
	})
})

var _ = Describe("[ics-e2e] [eit] Dynamic Provisioning using EIT enabled volume restricted to default worker pool,", func() {
	f := framework.NewDefaultFramework("ics-e2e-deploy")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		secretKey string
	)

	secretKey = os.Getenv("E2E_SECRET_ENCRYPTION_KEY")
	if secretKey == "" {
		secretKey = defaultSecret
	}

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		// Patch 'addon-vpc-file-csi-driver-configmap' to enable eit from operator
		cmData := map[string]interface{}{
			"data": map[string]string{
				"ENABLE_EIT":               "true",
				"EIT_ENABLED_WORKER_POOLS": "default",
			},
		}
		cmDataBytes, err := json.Marshal(cmData)
		if err != nil {
			panic(err)
		}

		var cm *v1.ConfigMap
		cm, err = cs.CoreV1().ConfigMaps("kube-system").Patch(context.TODO(), "addon-vpc-file-csi-driver-configmap", types.MergePatchType, cmDataBytes, metav1.PatchOptions{})
		if err != nil {
			panic(err)
		}

		fmt.Println("Updated ConfigMap 'addon-vpc-file-csi-driver-configmap': ", cm.Data)

		// Add wait for packages to be installed on the system
		fmt.Printf("Sleep for %s to install EIT packages...", waitForPackageInstallation)
		time.Sleep(waitForPackageInstallation)
		cm_status, err := cs.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "file-csi-driver-status", metav1.GetOptions{})
		if err != nil {
			panic(err)
		}
		eitEnabledWorkerNodes, exists := cm_status.Data["EIT_ENABLED_WORKER_NODES"]
		if !exists {
			fmt.Println("EIT_ENABLED_WORKER_NODES not found in ConfigMap")
			err = fmt.Errorf("unknown problem with 'file-csi-driver-status' configmap")
			panic(err)
		}
		// Print the content of EIT_ENABLED_WORKER_NODES
		fmt.Println("EIT_ENABLED_WORKER_NODES:")
		fmt.Println(eitEnabledWorkerNodes)
	})

	It("should create pv, pvc, deployment resources. Pod has affinity to nodes present in default worker pool and should pass", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			panic(labelerr)
		}
		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer fpointer.Close()

		var replicaCount = int32(3)
		pod := testsuites.PodDetails{
			Cmd:      "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 2; done",
			CmdExits: false,
			Volumes: []testsuites.VolumeDetails{
				{
					PVCName:       "ics-vol-dp2-",
					VolumeType:    "ibmc-vpc-file-eit",
					FSType:        "ibmshare",
					ClaimSize:     "10Gi",
					ReclaimPolicy: &reclaimPolicy,
					MountOptions:  []string{"rw"},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
			NodeSelector: map[string]string{
				"ibm-cloud.kubernetes.io/worker-pool-name": "default",
			},
		}
		test := testsuites.DynamicallyProvisioneDeployWithVolWRTest{
			Pod: pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
				ExpectedString02: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
			ReplicaCount: replicaCount,
		}
		test.Run(cs, ns)
		if _, err = fpointer.WriteString("VPC-FILE-CSI-TEST-EIT: VERIFYING PVC CREATE/DELETE RESTRICTED TO DEFAULT WORKER POOL : PASS\n"); err != nil {
			panic(err)
		}
	})
	AfterEach(func() {
		cmData := map[string]interface{}{
			"data": map[string]string{
				"ENABLE_EIT": "false",
			},
		}
		cmDataBytes, err := json.Marshal(cmData)
		if err != nil {
			panic(err)
		}

		_, err = cs.CoreV1().ConfigMaps("kube-system").Patch(context.TODO(), "addon-vpc-file-csi-driver-configmap", types.MergePatchType, cmDataBytes, metav1.PatchOptions{})
		if err != nil {
			panic(err)
		}

		// Add wait for packages to be uninstalled from the system
		fmt.Printf("Sleep for %s to uninstall EIT packages...", waitForPackageInstallation)
		time.Sleep(waitForPackageInstallation)
	})
})

var _ = Describe("[ics-e2e] [eit] Dynamic Provisioning on worker-pool where EIT is not enabled,", func() {
	f := framework.NewDefaultFramework("ics-e2e-deploy")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		secretKey string
	)

	secretKey = os.Getenv("E2E_SECRET_ENCRYPTION_KEY")
	if secretKey == "" {
		secretKey = defaultSecret
	}

	BeforeEach(func() {
		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		// Skip this if Multi-zone is disabled
		secondary_wp := os.Getenv("cluster_worker_pool")
		if secondary_wp == "" {
			if _, err = fpointer.WriteString("VPC-FILE-CSI-TEST-EIT: PROVISIONING DEPLOYMENT ON WP WHERE EIT IS NOT ENABLED MUST FAIL : SKIP\n"); err != nil {
				panic(err)
			}
			fpointer.Close()
			Skip("Skipping test because secondary worker pool is not set")
		}

		cs = f.ClientSet
		ns = f.Namespace
		// Patch 'addon-vpc-file-csi-driver-configmap' to enable eit from operator
		cmData := map[string]interface{}{
			"data": map[string]string{
				"ENABLE_EIT":               "true",
				"EIT_ENABLED_WORKER_POOLS": "default",
			},
		}
		cmDataBytes, err := json.Marshal(cmData)
		if err != nil {
			panic(err)
		}

		var cm *v1.ConfigMap
		cm, err = cs.CoreV1().ConfigMaps("kube-system").Patch(context.TODO(), "addon-vpc-file-csi-driver-configmap", types.MergePatchType, cmDataBytes, metav1.PatchOptions{})
		if err != nil {
			panic(err)
		}

		fmt.Println("Updated ConfigMap 'addon-vpc-file-csi-driver-configmap': ", cm.Data)

		// Add wait for packages to be installed on the system
		fmt.Printf("Sleep for %s to install EIT packages...", waitForPackageInstallation)
		time.Sleep(waitForPackageInstallation)
		cm_status, err := cs.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), "file-csi-driver-status", metav1.GetOptions{})
		if err != nil {
			panic(err)
		}
		eitEnabledWorkerNodes, exists := cm_status.Data["EIT_ENABLED_WORKER_NODES"]
		if !exists {
			fmt.Println("EIT_ENABLED_WORKER_NODES not found in ConfigMap")
			err = fmt.Errorf("unknown problem with 'file-csi-driver-status' configmap")
			panic(err)
		}
		// Print the content of EIT_ENABLED_WORKER_NODES
		fmt.Println("EIT_ENABLED_WORKER_NODES:")
		fmt.Println(eitEnabledWorkerNodes)
	})

	It("should create pv, pvc, deployment resources. Pod should be stuck in 'ContainerCreating' state", func() {
		payload := `{"metadata": {"labels": {"security.openshift.io/scc.podSecurityLabelSync": "false","pod-security.kubernetes.io/enforce": "privileged"}}}`
		_, labelerr := cs.CoreV1().Namespaces().Patch(context.TODO(), ns.Name, types.StrategicMergePatchType, []byte(payload), metav1.PatchOptions{})
		if labelerr != nil {
			panic(labelerr)
		}
		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		fpointer, err = os.OpenFile(testResultFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer fpointer.Close()
		secondary_wp := os.Getenv("cluster_worker_pool")

		var replicaCount = int32(3)
		pod := testsuites.PodDetails{
			Cmd:      "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 2; done",
			CmdExits: false,
			Volumes: []testsuites.VolumeDetails{
				{
					PVCName:       "ics-vol-dp2-",
					VolumeType:    "ibmc-vpc-file-eit",
					FSType:        "ibmshare",
					ClaimSize:     "10Gi",
					ReclaimPolicy: &reclaimPolicy,
					MountOptions:  []string{"rw"},
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
			NodeSelector: map[string]string{
				"ibm-cloud.kubernetes.io/worker-pool-name": secondary_wp,
			},
		}
		test := testsuites.DynamicallyProvisioneDeployWithVolWRTest{
			Pod: pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:              []string{"cat", "/mnt/test-1/data"},
				ExpectedString01: "hello world\n",
				ExpectedString02: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
			ReplicaCount: replicaCount,
		}
		test.RunShouldFail(cs, ns)
		if _, err = fpointer.WriteString("VPC-FILE-CSI-TEST-EIT: PROVISIONING DEPLOYMENT ON WP WHERE EIT IS NOT ENABLED MUST FAIL : PASS\n"); err != nil {
			panic(err)
		}
	})
	AfterEach(func() {
		// Skip this if Multi-zone is disabled
		secondary_wp := os.Getenv("cluster_worker_pool")
		if secondary_wp == "" {
			Skip("Skipping test because secondary worker pool is not set")
		}

		cmData := map[string]interface{}{
			"data": map[string]string{
				"ENABLE_EIT": "false",
			},
		}
		cmDataBytes, err := json.Marshal(cmData)
		if err != nil {
			panic(err)
		}

		_, err = cs.CoreV1().ConfigMaps("kube-system").Patch(context.TODO(), "addon-vpc-file-csi-driver-configmap", types.MergePatchType, cmDataBytes, metav1.PatchOptions{})
		if err != nil {
			panic(err)
		}

		// Add wait for packages to be uninstalled from the system
		fmt.Printf("Sleep for %s to uninstall EIT packages...", waitForPackageInstallation)
		time.Sleep(waitForPackageInstallation)
	})
})
