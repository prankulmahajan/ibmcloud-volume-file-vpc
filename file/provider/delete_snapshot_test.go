/**
 * Copyright 2025 IBM Corp.
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

// Package provider ...
package provider

import (
	"errors"
	"testing"

	userError "github.com/IBM/ibmcloud-volume-file-vpc/common/messages"
	serviceFakes "github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/vpcfilevolume/fakes"
	"github.com/IBM/ibmcloud-volume-interface/lib/provider"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestDeleteSnapshot(t *testing.T) {
	//var err error
	logger, teardown := GetTestLogger(t)
	defer teardown()
	userError.MessagesEn = userError.InitMessages()

	var (
		snapshotService *serviceFakes.SnapshotManager
	)

	testCases := []struct {
		testCaseName     string
		providerSnapshot *provider.Snapshot
		setup            func()

		skipErrTest bool
		expectedErr string
		backendErr  string

		verify func(t *testing.T, err error)
	}{
		{
			testCaseName: "Success Case",
			providerSnapshot: &provider.Snapshot{
				VolumeID:   "16f293bf-test-4bff-816f-e199c0c65db5",
				SnapshotID: "16f293bf-test-4bff-816f-e199c0c65db6",
			},
			verify: func(t *testing.T, err error) {
				assert.Nil(t, err)
			},
		}, {
			testCaseName: "False positive: No Snapshot being sent",
			providerSnapshot: &provider.Snapshot{
				VolumeID:   "16f293bf-test-4bff-816f-e199c0c65db5",
				SnapshotID: "",
			},
			expectedErr: "{Trace Code:16f293bf-test-4bff-816f-e199c0c65db5, Code:share_snapshot_not_found, Description: Snapshot does not exist.Failed to delete '' snapshot ID from share ID '16f293bf-test-4bff-816f-e199c0c65db5'}",
			backendErr:  "Trace Code:16f293bf-test-4bff-816f-e199c0c65db5, Code:share_snapshot_not_found, Description: Snapshot does not exist",
			verify: func(t *testing.T, err error) {
				assert.NotNil(t, err)
			},
		}, {
			testCaseName: "NonExisting snapshot ID",
			providerSnapshot: &provider.Snapshot{
				VolumeID:   "16f293bf-test-4bff-816f-e199c0c65db5",
				SnapshotID: "16f293bf-test-4bff-816f-e199c0c65db6",
			},
			verify: func(t *testing.T, err error) {
				assert.Nil(t, err)
			},
		},
	}

	for _, testcase := range testCases {
		userError.MessagesEn = userError.InitMessages()
		t.Run(testcase.testCaseName, func(t *testing.T) {
			vpcs, uc, sc, err := GetTestOpenSession(t, logger)
			assert.NotNil(t, vpcs)
			assert.NotNil(t, uc)
			assert.NotNil(t, sc)
			assert.Nil(t, err)

			snapshotService = &serviceFakes.SnapshotManager{}
			assert.NotNil(t, snapshotService)
			uc.SnapshotServiceReturns(snapshotService)

			if testcase.expectedErr != "" {
				snapshotService.DeleteSnapshotReturns(errors.New(testcase.backendErr))
			} else {
				snapshotService.DeleteSnapshotReturns(nil)
			}
			err = vpcs.DeleteSnapshot(testcase.providerSnapshot)

			if testcase.expectedErr != "" {
				assert.NotNil(t, err)
				logger.Info("Error details", zap.Reflect("Error details", err.Error()))
				assert.Equal(t, testcase.expectedErr, err.Error())
			}

			if testcase.verify != nil {
				testcase.verify(t, err)
			}
		})
	}
}

func TestDeleteSnapshotTwo(t *testing.T) {
	//var err error
	logger, teardown := GetTestLogger(t)
	defer teardown()

	var (
		snapshotService *serviceFakes.SnapshotManager
	)
	providerSnapshot := &provider.Snapshot{
		VolumeID:   "16f293bf-test-4bff-816f-e199c0c65db5",
		SnapshotID: "16f293bf-test-4bff-816f-e199c0c65db6",
	}

	vpcs, uc, sc, err := GetTestOpenSession(t, logger)
	assert.NotNil(t, vpcs)
	assert.NotNil(t, uc)
	assert.NotNil(t, sc)
	assert.Nil(t, err)
	snapshotService = &serviceFakes.SnapshotManager{}
	assert.NotNil(t, snapshotService)
	uc.SnapshotServiceReturns(snapshotService)

	snapshotService.DeleteSnapshotReturns(errors.New("not_found"))

	err = vpcs.DeleteSnapshot(providerSnapshot)
	assert.NotNil(t, err)

	snapshotService.DeleteSnapshotReturns(errors.New("failedToDeleteSnapshot"))

	err = vpcs.DeleteSnapshot(providerSnapshot)
	assert.NotNil(t, err)

	snapshotService.DeleteSnapshotReturns(errors.New("failedToDeleteSnapshot"))

	err = vpcs.DeleteSnapshot(providerSnapshot)
	assert.NotNil(t, err)
}
