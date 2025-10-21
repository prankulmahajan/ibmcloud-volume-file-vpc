#!/bin/bash

#/******************************************************************************
# Copyright 2024 IBM Corp.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# *****************************************************************************/

GOPATH=$GOPATH
VPC_FILE_CSI_HOME="$GOPATH/src/github.com/IBM/ibmcloud-volume-file-vpc"
E2E_TEST_SETUP="$VPC_FILE_CSI_HOME/e2e-setup.out"
E2E_TEST_RESULT="$VPC_FILE_CSI_HOME/e2e-test.out"
export E2E_TEST_RESULT=$E2E_TEST_RESULT
export E2E_TEST_SETUP=$E2E_TEST_SETUP
export cluster_worker_pool="e2etest-vpc"
SECRET_CREATION_WAIT=600 #seconds

rm -f $E2E_TEST_RESULT
rm -f $E2E_TEST_SETUP

IC_LOGIN="false"
PVCCOUNT="single"

UNKOWNPARAM=()
while [[ $# -gt 0 ]]; do
	key="$1"
	case $key in
		-l|--login)
		IC_LOGIN="true"
		shift
		;;
		-e|--env)
		TEST_ENV="$2"
		shift
		shift
		;;
		-r|--region)
		REGION="$2"
		shift
		shift
		;;
		--addon-version)
		e2e_addon_version="$2"
		shift
		shift
		;;
		
		-tp| --use-trusted-profile)
		e2e_tp="$2"
		shift;
		shift
		;;

		--run-eit-test-cases)
		e2e_eit_test_case="$2"
		shift
		shift
		;;
		--run-rfs-test-cases)
		e2e_rfs_test_case="$2"
		shift
		shift
		;;
    		*)
    		UNKOWNPARAM+=("$1")
    		shift
    		;;
	esac
done

if [[ "$IC_LOGIN" == "true" ]]; then
	echo "Kube Config already exported!!!"
fi

if [[ "$IC_LOGIN" != "true" ]]; then
   echo "Error: Not logged into IBM Cloud!!!"
   echo "VPC-FILE-CSI-TEST: Cluster-Setup: FAILED" > $E2E_TEST_RESULT
   exit 1
fi

# Validate that ibm-cloud-credentials is created
wait_for_secret() {
    echo
    echo "Waiting up to ${SECRET_CREATION_WAIT}s for ibm-cloud-credentials to appear..."

    local elapsed=0
    while [[ $elapsed -lt ${SECRET_CREATION_WAIT} ]]; do
      if kubectl get secret ibm-cloud-credentials -n kube-system; then
        echo "ibm-cloud-credentials found in namespace kube-system."
        return 0
      fi

      sleep 5
      ((elapsed+=5))
    done

    echo "ibm-cloud-credentials was not created within ${SECRET_CREATION_WAIT}s."
    return 1
}

function check_trusted_profile_status {
	set -x
	expected_profile_id=""
    if [[ "$e2e_tp" == "true" ]]; then
		if [[ "$TEST_ENV" == "stage" ]]; then
            expected_profile_id=$STAGE_TRUSTED_PROFILE_ID
        else
            expected_profile_id=$PROD_TRUSTED_PROFILE_ID
        fi
		echo "************************Trusted Profile Check ***************************" >> $E2E_TEST_SETUP
        # Secret existence
        wait_for_secret
        secret_json=$(kubectl get secret ibm-cloud-credentials -n kube-system -o json)
        encoded=$(jq -r '.data["ibm-credentials.env"]' <<< "$secret_json")
        decoded=$(base64 --decode <<< "$encoded")
        profileID=$(echo $decoded | grep IBMCLOUD_PROFILEID | cut -d'=' -f3-)
        if [[ "$profileID" == "$expected_profile_id" ]]; then
            echo -e "VPC-FILE-CSI-TEST: USING TRUSTED_PROFILE: TRUE" >> $E2E_TEST_SETUP
			echo "***************************************************" >> $E2E_TEST_SETUP
        else
            echo -e "VPC-FILE-CSI-TEST: USING TRUSTED_PROFILE: FAILED" >> $E2E_TEST_SETUP
			echo "***************************************************" >> $E2E_TEST_SETUP
            exit 1
        fi
    fi
}

echo "**********VPC-File-Volume-Tests**********" > $E2E_TEST_RESULT
echo "********** E2E Test Details **********" > $E2E_TEST_SETUP
echo -e "StartTime   : $(date "+%F-%T")" >> $E2E_TEST_SETUP

CLUSTER_DETAIL=$(kubectl get cm cluster-info -n kube-system -o jsonpath='{.data.cluster-config\.json}' |\
		 grep -v -e 'crn' -e 'master_public_url' -e 'master_url'); rc=$?
if [[ $rc -ne 0 ]]; then
	echo -e "Error       : Setup failed" >> $E2E_TEST_SETUP
	echo -e "Error       : Unable to connect to the cluster" >> $E2E_TEST_SETUP
	echo -e "Error       : Unbale to execute e2e test!"
	echo -e "VPC-FILE-CSI-TEST: VPC-File-Volume-Tests: FAILED" >> $E2E_TEST_RESULT
	exit 1
fi

check_trusted_profile_status

CLUSTER_KUBE_DETAIL=$(kubectl get nodes -o jsonpath="{range .items[*]}{.metadata.name}:{.status.nodeInfo.kubeletVersion}:{.status.nodeInfo.osImage} {'\n'}"); rc=$?
echo -e "***************** Cluster Details ******************" >> $E2E_TEST_SETUP
echo -e "$CLUSTER_DETAIL" >> $E2E_TEST_SETUP
echo -e "----------------------------------------------------" >> $E2E_TEST_SETUP

echo -e "----------------------------------------------------" >> $E2E_TEST_SETUP
echo -e "$CLUSTER_KUBE_DETAIL" >> $E2E_TEST_SETUP
echo -e "----------------------------------------------------" >> $E2E_TEST_SETUP
echo -e "$DRIVER_DETAILS" >> $E2E_TEST_SETUP
echo -e "Addon Version: $CLUSTER_ADDON_VER" >> $E2E_TEST_SETUP
echo "***************************************************" >> $E2E_TEST_SETUP

err_msg1=""
err_msg2=""
DRIVER_PODS=$(kubectl get pods -n kube-system | grep 'ibm-vpc-file-csi-controlle' | grep 'Running'); rc=$?
if [[ $rc -ne 0 ]]; then
    err_msg1="Error       : Controller not active"
	echo -e "VPC-FILE-CSI-TEST: VERIFYING VPC FILE CSI DRIVER HEALTH: FAILED" >> $E2E_TEST_SETUP
	echo "***************************************************" >> $E2E_TEST_SETUP
	DRIVER_DETAILS=$(kubectl describe pod -n kube-system ibm-vpc-file-csi-controller | sed -n '/Events/,$p'); 
	echo -e "\nDRIVER DETAILS = $DRIVER_DETAILS" >> $E2E_TEST_SETUP
	echo "***************************************************" >> $E2E_TEST_SETUP
else
	echo -e "VPC-FILE-CSI-TEST: VERIFYING VPC FILE CSI DRIVER HEALTH: PASS" >> $E2E_TEST_SETUP
	echo "***************************************************" >> $E2E_TEST_SETUP
    DRIVER_DETAILS=$(kubectl get deployment -n kube-system ibm-vpc-file-csi-controller -o jsonpath="{range .spec.template.spec.containers[*]}{.name}:{.image}{'\n'}"); rc=$?
	echo -e "\nDRIVER DETAILS = $DRIVER_DETAILS" >> $E2E_TEST_SETUP
	echo "***************************************************" >> $E2E_TEST_SETUP
fi

DRIVER_PODS=$(kubectl get pods -n kube-system | grep 'ibm-vpc-file-csi-node' | grep 'Running'); rc=$?
if [[ $rc -ne 0 ]]; then
    err_msg2="Error       : Node server not active"
	echo "***************************************************" >> $E2E_TEST_SETUP
	DRIVER_DETAILS=$(kubectl describe pod -n kube-system ibm-vpc-file-csi-node | sed -n '/Events/,$p');
	echo -e "\nDRIVER DETAILS = $DRIVER_DETAILS" >> $E2E_TEST_SETUP
	echo "***************************************************" >> $E2E_TEST_SETUP
fi

if [[ -n "$err_msg1" || -n "$err_msg2" ]]; then
	echo -e "Error       : Setup failed" >> $E2E_TEST_SETUP
	[[ -n "$err_msg1" ]] && echo -e "$err_msg1" >> $E2E_TEST_SETUP
	[[ -n "$err_msg2" ]] && echo -e "$err_msg2" >> $E2E_TEST_SETUP
	echo "***************************************************" >> $E2E_TEST_SETUP
	echo -e "VPC-FILE-CSI-TEST: VPC-File-Volume-Tests: FAILED" >> $E2E_TEST_RESULT
	exit 1
fi

set +e
# check mandatory variables
echo "Running E2E for region: [$TEST_ENV]"
echo "                  Path: `pwd`"

# Set storage class based on addon version
# To be removed once 1.2 addon version is unsupoo
version_ge() {
  [ "$(printf '%s\n' "$1" "$2" | sort -V | head -n1)" = "$2" ]
}

if version_ge "$e2e_addon_version" "2.0"; then
	export SC="ibmc-vpc-file-min-iops"
	export SC_RETAIN="ibmc-vpc-file-retain-500-iops"
else
	export SC="ibmc-vpc-file-dp2"
	export SC_RETAIN="ibmc-vpc-file-retain-dp2"
fi

# E2E Execution
go clean -modcache
export GO111MODULE=on
go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo@v2.21.0
set +e

# Non EIT based tests
ginkgo -v -nodes=1 --focus="\[ics-e2e\] \[sc\]" ./e2e -- -e2e-verify-service-account=false
rc1=$?
echo "Exit status for basic volume test: $rc1"

ginkgo -v -nodes=1 --focus="\[ics-e2e\] \[resize\] \[pv\]" ./e2e -- -e2e-verify-service-account=false
rc2=$?
echo "Exit status for resize volume test: $rc2"

# RFS Profile tests
if [[ "$e2e_rfs_test_case" == "true" ]]; then
	ginkgo -v -nodes=1 --focus="\[ics-e2e\] \[sc_rfs\]" ./e2e -- -e2e-verify-service-account=false
	rc4=$?
	echo "Exit status for RFS Profile volume test: $rc4"
	
	if [[ $rc4 -eq 0 ]]; then
		echo -e "VPC-FILE-CSI-TEST-RFS: VPC-File-RFS-Volume-Tests: PASS" >> $E2E_TEST_RESULT
	else
		echo -e "VPC-FILE-CSI-TEST-RFS: VPC-File-RFS-Volume-Tests: FAILED" >> $E2E_TEST_RESULT
	fi
else
	echo -e "VPC-FILE-CSI-TEST-RFS: VPC-File-Volume-Tests: SKIP" >> $E2E_TEST_RESULT
fi

if [[ $rc1 -eq 0 && $rc2 -eq 0 ]]; then
	echo -e "VPC-FILE-CSI-TEST: VPC-File-Volume-Tests: PASS" >> $E2E_TEST_RESULT
else
	echo -e "VPC-FILE-CSI-TEST: VPC-File-Volume-Tests: FAILED" >> $E2E_TEST_RESULT
fi

# EIT based tests

if [[ "$e2e_eit_test_case" == "true" && "$CLUSTER_KUBE_VER_TRIM=" != "4.15" ]]; then
	# EIT based tests (To be run only for addon version >=2.0)
	if version_ge "$e2e_addon_version" "2.0"; then
		ginkgo -v -nodes=1 --focus="\[ics-e2e\] \[eit\]" ./e2e -- -e2e-verify-service-account=false
		rc3=$?
		echo "Exit status for EIT volume test: $rc3"
	else
		echo "Conditions to run EIT test cases did not pass..."
		rc3=1
	fi

	if [[ $rc3 -eq 0 ]]; then
		echo -e "VPC-FILE-CSI-TEST-EIT: VPC-File-EIT-Volume-Tests: PASS" >> $E2E_TEST_RESULT
	else
		echo -e "VPC-FILE-CSI-TEST-EIT: VPC-File-EIT-Volume-Tests: FAILED" >> $E2E_TEST_RESULT
	fi
else
	echo -e "VPC-FILE-CSI-TEST-EIT: VPC-File-EIT-Volume-Tests: SKIP" >> $E2E_TEST_RESULT
fi

# Publish final reports
grep  'VPC-FILE-CSI-TEST: VPC-File-Volume-Tests: FAILED' $E2E_TEST_RESULT; ex1=$?
grep  'VPC-FILE-CSI-TEST-EIT: VPC-File-EIT-Volume-Tests: FAILED' $E2E_TEST_RESULT; ex2=$?

if [[ $ex1 -eq 0 || $ex2 -eq 0 ]]; then
	exit 1
else
	exit 0
fi
