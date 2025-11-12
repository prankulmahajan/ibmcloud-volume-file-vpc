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
	"strings"
	"testing"
	"time"

	userError "github.com/IBM/ibmcloud-volume-file-vpc/common/messages"
	"github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/models"
	snapshotServiceFakes "github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/vpcfilevolume/fakes"
	"github.com/IBM/ibmcloud-volume-interface/lib/provider"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestListSnapshots(t *testing.T) {
	//var err error
	logger, teardown := GetTestLogger(t)
	userError.MessagesEn = userError.InitMessages()
	defer teardown()

	var (
		snapshotService *snapshotServiceFakes.SnapshotManager
	)
	timeNow := time.Now()

	testCases := []struct {
		testCaseName string
		snapshotList *models.SnapshotList

		limit int
		start string
		tags  map[string]string

		setup func()

		skipErrTest bool
		expectedErr string
		backendErr  string

		verify func(t *testing.T, next_token string, snapshots *provider.SnapshotList, err error)
	}{
		{
			testCaseName: "Filter by source_volume.id",
			snapshotList: &models.SnapshotList{
				First: &models.HReference{Href: "https://eu-gb.iaas.cloud.ibm.com/v1/snapshots?start=16f293bf-test-4bff-816f-e199c0c65db5\u0026limit=50\u0026source_volume.id=1234"},
				Next:  nil,
				Limit: 50,
				Snapshots: []*models.Snapshot{
					{
						ID:             "16f293bf-test-4bff-816f-e199c0c65db5",
						Name:           "test-snapshot-name1",
						LifecycleState: snapshotReadyState,
						CreatedAt:      &timeNow,
					}, {
						ID:             "16f293bf-test-4bff-816f-e199c0c65db6",
						Name:           "test-snapshot-name2",
						LifecycleState: snapshotReadyState,
						CreatedAt:      &timeNow,
					},
				},
			},
			tags: map[string]string{
				"source_volume.id": "1234",
			},
			verify: func(t *testing.T, next_token string, snapshots *provider.SnapshotList, err error) {
				assert.NotNil(t, snapshots.Snapshots)
				assert.Equal(t, next_token, snapshots.Next)
				assert.Nil(t, err)
			},
		}, {
			testCaseName: "Filter by source_volume.id, 1 entry per page",
			snapshotList: &models.SnapshotList{
				First: &models.HReference{Href: "https://eu-gb.iaas.cloud.ibm.com/v1/snapshots?start=16f293bf-test-4bff-816f-e199c0c65db5\u0026limit=1\u0026source_volume.id=1234"},
				Next:  &models.HReference{Href: "https://eu-gb.iaas.cloud.ibm.com/v1/snapshots?start=23b154fr-test-4bff-816f-f213s1y34gj8\u0026limit=1\u0026source_volume.id=1234"},
				Limit: 1,
				Snapshots: []*models.Snapshot{
					{
						ID:             "16f293bf-test-4bff-816f-e199c0c65db5",
						Name:           "test-snapshot-name1",
						LifecycleState: snapshotReadyState,
						CreatedAt:      &timeNow,
					}, {
						ID:             "16f293bf-test-4bff-816f-e199c0c65db6",
						Name:           "test-snapshot-name2",
						LifecycleState: snapshotReadyState,
						CreatedAt:      &timeNow,
					},
				},
			},
			tags: map[string]string{
				"source_volume.id": "1234",
			},
			limit: 1,
			verify: func(t *testing.T, next_token string, snapshots *provider.SnapshotList, err error) {
				assert.NotNil(t, snapshots.Snapshots)
				assert.Equal(t, next_token, snapshots.Next)
				assert.Nil(t, err)
			},
		}, {
			testCaseName: "Filter by source_volume.id: no volume found", // Filter by zone where no volume is present
			snapshotList: &models.SnapshotList{
				First:     &models.HReference{Href: "https://eu-gb.iaas.cloud.ibm.com/v1/snapshots?limit=50\u0026source_volume.id=1234"},
				Next:      nil,
				Limit:     50,
				Snapshots: []*models.Snapshot{},
			},
			tags: map[string]string{
				"source_volume.id": "1234",
			},
			verify: func(t *testing.T, next_token string, snapshots *provider.SnapshotList, err error) {
				assert.Nil(t, snapshots.Snapshots)
				assert.Equal(t, next_token, snapshots.Next)
				assert.Nil(t, err)
			},
		}, {
			testCaseName: "Filter by name",
			snapshotList: &models.SnapshotList{
				First: &models.HReference{Href: "https://eu-gb.iaas.cloud.ibm.com/v1/snapshots?start=16f293bf-test-4bff-816f-e199c0c65db5\u0026limit=50"},
				Next:  nil,
				Limit: 50,
				Snapshots: []*models.Snapshot{
					{
						ID:             "16f293bf-test-4bff-816f-e199c0c65db5",
						Name:           "test-snapshot-name1",
						LifecycleState: "stable",
						CreatedAt:      &timeNow,
					},
				},
			},
			tags: map[string]string{
				"name":             "test-snapshot-name1",
				"source_volume.id": "1234",
			},
			verify: func(t *testing.T, next_token string, snapshots *provider.SnapshotList, err error) {
				assert.NotNil(t, snapshots.Snapshots)
				assert.Equal(t, next_token, snapshots.Next)
				assert.Nil(t, err)
			},
		}, {
			testCaseName: "Filter by name: snapshot not found",
			tags: map[string]string{
				"name":             "test-snapshot-name1",
				"source_volume.id": "1234",
			},
			verify: func(t *testing.T, next_token string, snapshots *provider.SnapshotList, err error) {
				assert.Nil(t, snapshots.Snapshots)
				assert.Nil(t, err)
			},
		}, {
			testCaseName: "List SnapShot fails",
			tags: map[string]string{
				"name":             "test-snapshot-name1",
				"source_volume.id": "1234",
			},
			start:       "testID",
			expectedErr: "{Trace Code:16f293bf-test-4bff-816f-e199c0c65db5, Code:bad_field, Description: The `start` parameter is invalid, item no longer found.The snapshot ID 'testID' specified in the start parameter of the list volume call could not be found.}",
			backendErr:  "Trace Code:16f293bf-test-4bff-816f-e199c0c65db5, Code:bad_field, Description: The `start` parameter is invalid, item no longer found",
			verify: func(t *testing.T, next_token string, snapshots *provider.SnapshotList, err error) {
				assert.Nil(t, snapshots)
				assert.NotNil(t, err)
			},
		}, {
			testCaseName: "List all snapshots",
			snapshotList: &models.SnapshotList{
				First: &models.HReference{Href: "https://eu-gb.iaas.cloud.ibm.com/v1/snapshots?start=16f293bf-test-4bff-816f-e199c0c65db5\u0026limit=50"},
				Next:  nil,
				Limit: 50,
				Snapshots: []*models.Snapshot{
					{
						ID:             "16f293bf-test-4bff-816f-e199c0c65db5",
						Name:           "test-snapshot-name1",
						LifecycleState: "stable",
						CreatedAt:      &timeNow,
					}, {
						ID:             "16f293bf-test-4bff-816f-e199c0c65db6",
						Name:           "test-snapshot-name2",
						LifecycleState: "stable",
						CreatedAt:      &timeNow,
					},
				},
			},
			tags: map[string]string{
				"source_volume.id": "1234",
			},
			verify: func(t *testing.T, next_token string, snapshots *provider.SnapshotList, err error) {
				assert.NotNil(t, snapshots.Snapshots)
				assert.Equal(t, next_token, snapshots.Next)
				assert.Nil(t, err)
			},
		}, {
			testCaseName: "Unexpected format of 'Next' parameter in ListVolumes response",
			snapshotList: &models.SnapshotList{
				First: &models.HReference{Href: "https://eu-gb.iaas.cloud.ibm.com/v1/volumes?start=16f293bf-test-4bff-816f-e199c0c65db5\u0026limit=50"},
				Next:  &models.HReference{Href: "https://eu-gb.iaas.cloud.ibm.com/v1/volumes?invalid=16f293bf-test-4bff-816f-e199c0c65db5\u0026limit=50"},
				Limit: 1,
				Snapshots: []*models.Snapshot{
					{
						ID:             "16f293bf-test-4bff-816f-e199c0c65db5",
						Name:           "test-snapshot-name1",
						LifecycleState: "stable",
						CreatedAt:      &timeNow,
					}, {
						ID:             "16f293bf-test-4bff-816f-e199c0c65db6",
						Name:           "test-snapshot-name2",
						LifecycleState: "stable",
						CreatedAt:      &timeNow,
					},
				},
			},
			limit: 1,
			verify: func(t *testing.T, next_token string, snapshots *provider.SnapshotList, err error) {
				assert.NotNil(t, snapshots.Snapshots)
				assert.Equal(t, next_token, snapshots.Next)
				assert.Nil(t, err)
			},
		}, {
			testCaseName: "Invalid limit value",
			limit:        -1,
			verify: func(t *testing.T, next_token string, snapshots *provider.SnapshotList, err error) {
				assert.Nil(t, snapshots)
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), "The value '-1' specified in the limit parameter of the list snapshot call is not valid")
				}
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

			snapshotService = &snapshotServiceFakes.SnapshotManager{}
			assert.NotNil(t, snapshotService)
			uc.SnapshotServiceReturns(snapshotService)

			if testcase.expectedErr != "" {
				snapshotService.ListSnapshotsReturns(testcase.snapshotList, errors.New(testcase.backendErr))
			} else {
				snapshotService.ListSnapshotsReturns(testcase.snapshotList, nil)
			}
			snapshots, err := vpcs.ListSnapshots(testcase.limit, testcase.start, testcase.tags)
			logger.Info("SnapshotsList details", zap.Reflect("SnapshotsList", snapshots))

			if testcase.expectedErr != "" {
				assert.NotNil(t, err)
				logger.Info("Error details", zap.Reflect("Error details", err.Error()))
				assert.Equal(t, testcase.expectedErr, err.Error())
			}

			if testcase.verify != nil {
				var next string
				if testcase.snapshotList != nil {
					if testcase.snapshotList.Next != nil {
						// "Next":{"href":"https://eu-gb.iaas.cloud.ibm.com/v1/snapshots?start=3e898aa7-ac71-4323-952d-a8d741c65a68\u0026limit=1\u0026source_volume.id=1234"}
						if strings.Contains(testcase.snapshotList.Next.Href, "start=") {
							next = strings.Split(strings.Split(testcase.snapshotList.Next.Href, "start=")[1], "\u0026")[0]
						}
					}
				}
				testcase.verify(t, next, snapshots, err)
				if snapshots != nil && snapshots.Snapshots != nil {
					for index, snap := range snapshots.Snapshots {
						assert.Equal(t, testcase.snapshotList.Snapshots[index].ID, snap.SnapshotID)
					}
				}
			}
		})
	}
}
