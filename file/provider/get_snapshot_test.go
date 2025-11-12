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
	"time"

	userError "github.com/IBM/ibmcloud-volume-file-vpc/common/messages"
	"github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/models"
	serviceFakes "github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/vpcfilevolume/fakes"
	"github.com/IBM/ibmcloud-volume-interface/lib/provider"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestGetSnapshot(t *testing.T) {
	//var err error
	logger, teardown := GetTestLogger(t)
	defer teardown()
	userError.MessagesEn = userError.InitMessages()

	var (
		snapshotService *serviceFakes.SnapshotManager
	)
	timeNow := time.Now()

	testCases := []struct {
		testCaseName string

		snapshotID     string
		sourceVolumeID string
		baseSnapshot   *models.Snapshot
		setup          func()

		skipErrTest bool
		expectedErr string
		backendErr  string

		verify func(t *testing.T, snapshotResponse *provider.Snapshot, err error)
	}{
		{
			testCaseName:   "OK",
			sourceVolumeID: "16f293bf-test-4bff-816f-e199c0c65db5",
			snapshotID:     "16f293bf-test-4bff-816f-e199c0c65db5",
			baseSnapshot: &models.Snapshot{
				ID:             "16f293bf-test-4bff-816f-e199c0c65db5",
				Name:           "test-snapshot-name",
				LifecycleState: snapshotReadyState,
				CreatedAt:      &timeNow,
			},
			verify: func(t *testing.T, snapshotResponse *provider.Snapshot, err error) {
				assert.NotNil(t, snapshotResponse)
				assert.Nil(t, err)
			},
		}, {
			testCaseName:   "Wrong snapshot ID",
			sourceVolumeID: "16f293bf-test-4bff-816f-e199c0c65db5",
			snapshotID:     "Wrong snapshot ID",
			baseSnapshot: &models.Snapshot{
				ID:             "wrong-wrong-id",
				Name:           "test-snapshot-name",
				LifecycleState: snapshotReadyState,
				CreatedAt:      &timeNow,
			},
			expectedErr: "{Trace Code:16f293bf-test-4bff-816f-e199c0c65db5, Code:share_snapshot_not_found, Description: Snapshot does not exist.A snapshot with the specified snapshot ID 'Wrong snapshot ID' and share ID '[16f293bf-test-4bff-816f-e199c0c65db5]' could not be found.}",
			backendErr:  "Trace Code:16f293bf-test-4bff-816f-e199c0c65db5, Code:share_snapshot_not_found, Description: Snapshot does not exist",
			verify: func(t *testing.T, snapshotResponse *provider.Snapshot, err error) {
				assert.Nil(t, snapshotResponse)
				assert.NotNil(t, err)
			},
		},
	}

	for _, testcase := range testCases {
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
				snapshotService.GetSnapshotReturns(testcase.baseSnapshot, errors.New(testcase.backendErr))
			} else {
				snapshotService.GetSnapshotReturns(testcase.baseSnapshot, nil)
			}
			snapshot, err := vpcs.GetSnapshot(testcase.snapshotID, testcase.sourceVolumeID)
			logger.Info("Snapshot details", zap.Reflect("snapshot", snapshot))

			if testcase.expectedErr != "" {
				assert.NotNil(t, err)
				logger.Info("Error details", zap.Reflect("Error details", err.Error()))
				assert.Equal(t, testcase.expectedErr, err.Error())
			}

			if testcase.verify != nil {
				testcase.verify(t, snapshot, err)
			}
		})
	}
}

func TestGetSnapshotByName(t *testing.T) {
	//var err error
	logger, teardown := GetTestLogger(t)
	defer teardown()
	userError.MessagesEn = userError.InitMessages()

	var (
		snapshotService *serviceFakes.SnapshotManager
	)
	timeNow := time.Now()

	testCases := []struct {
		testCaseName   string
		snapshotName   string
		sourceVolumeID string
		baseSnapshot   *models.Snapshot

		setup func()

		skipErrTest bool
		expectedErr string
		backendErr  string

		verify func(t *testing.T, snapshotResponse *provider.Snapshot, err error)
	}{
		{
			testCaseName:   "OK",
			sourceVolumeID: "16f293bf-test-4bff-816f-e199c0c65db5",
			snapshotName:   "Test snapshot",
			baseSnapshot: &models.Snapshot{
				ID:             "wrong-wrong-id",
				Name:           "test-snapshot-name",
				LifecycleState: snapshotReadyState,
				CreatedAt:      &timeNow,
			},
			verify: func(t *testing.T, snapshotResponse *provider.Snapshot, err error) {
				assert.NotNil(t, snapshotResponse)
				assert.Nil(t, err)
			},
		}, {
			testCaseName:   "Wrong snapshot ID",
			sourceVolumeID: "16f293bf-test-4bff-816f-e199c0c65db5",
			snapshotName:   "Wrong snapshot name",
			baseSnapshot: &models.Snapshot{
				ID:             "wrong-wrong-id",
				Name:           "test-snapshot-name",
				LifecycleState: snapshotReadyState,
				CreatedAt:      &timeNow,
			},
			expectedErr: "{Trace Code:16f293bf-test-4bff-816f-e199c0c65db5, Code:share_snapshot_not_found, Description: Snapshot does not exist.A snapshot with the specified snapshot name 'Wrong snapshot name' and share ID '[16f293bf-test-4bff-816f-e199c0c65db5]' could not be found.}",
			backendErr:  "Trace Code:16f293bf-test-4bff-816f-e199c0c65db5, Code:share_snapshot_not_found, Description: Snapshot does not exist",
			verify: func(t *testing.T, snapshotResponse *provider.Snapshot, err error) {
				assert.Nil(t, snapshotResponse)
				assert.NotNil(t, err)
			},
		}, {
			testCaseName: "Empty snapshot name",
			snapshotName: "",
			expectedErr:  "{Code:ErrorRequiredFieldMissing, Description:[SnapshotName] is required to complete the operation., RC:400}",
			backendErr:   "Code:ErrorRequiredFieldMissing, Description:[SnapshotName] is required to complete the operation., RC:400",
			verify: func(t *testing.T, snapshotResponse *provider.Snapshot, err error) {
				assert.Nil(t, snapshotResponse)
				assert.NotNil(t, err)
			},
		},
	}

	for _, testcase := range testCases {
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
				snapshotService.GetSnapshotByNameReturns(testcase.baseSnapshot, errors.New(testcase.backendErr))
			} else {
				snapshotService.GetSnapshotByNameReturns(testcase.baseSnapshot, nil)
			}
			snapshot, err := vpcs.GetSnapshotByName(testcase.snapshotName, testcase.sourceVolumeID)
			logger.Info("Snapshot details", zap.Reflect("volume", snapshot))

			if testcase.expectedErr != "" {
				assert.NotNil(t, err)
				logger.Info("Error details", zap.Reflect("Error details", err.Error()))
				assert.Equal(t, testcase.expectedErr, err.Error())
			}

			if testcase.verify != nil {
				testcase.verify(t, snapshot, err)
			}
		})
	}
}
