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

// Package vpcfilevolume_test ...
package vpcfilevolume_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/models"
	"github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/riaas/test"
	"github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/vpcfilevolume"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestListSnapshots(t *testing.T) {
	// Setup new style zap logger
	logger, _ := GetTestContextLogger()
	defer logger.Sync()

	testCases := []struct {
		name string

		// Response
		status  int
		content string

		limit   int
		start   string
		filters *models.LisSnapshotFilters

		// Expected return
		expectErr string
		verify    func(t *testing.T, snapshots *models.SnapshotList, err error)
		muxVerify func(*testing.T, *http.Request)
	}{
		{
			name:   "Verify that the correct endpoint is invoked",
			status: http.StatusNoContent,
		}, {
			name:      "Verify that a 404 is returned to the caller",
			status:    http.StatusNotFound,
			content:   "{\"errors\":[{\"message\":\"testerr\",\"Code\":\"share_not_found\"}], \"trace\":\"2af63776-4df7-4970-b52d-4e25676ec0e4\"}",
			expectErr: "Trace Code:2af63776-4df7-4970-b52d-4e25676ec0e4, Code:share_not_found, Description:testerr, RC:404 Not Found",
		}, {
			name:   "Verify that limit is added to the query",
			limit:  12,
			status: http.StatusNoContent,
			muxVerify: func(t *testing.T, r *http.Request) {
				expectedValues := url.Values{"limit": []string{"12"}, "version": []string{models.APIVersion}}
				actualValues := r.URL.Query()
				assert.Equal(t, expectedValues, actualValues)
			},
		}, {
			name:   "Verify that start is added to the query",
			start:  "x-y-z",
			status: http.StatusNoContent,
			muxVerify: func(t *testing.T, r *http.Request) {
				expectedValues := url.Values{"start": []string{"x-y-z"}, "version": []string{models.APIVersion}}
				actualValues := r.URL.Query()
				assert.Equal(t, expectedValues, actualValues)
			},
		}, {
			name: "Verify that snapshot name is added to the query",
			filters: &models.LisSnapshotFilters{
				Name: "testname",
			},
			status:  http.StatusOK,
			content: "{\"first\":{\"href\":\"href1\"},\"limit\":50,\"snapshots\":[{\"id\":\"testname\",\"name\":\"testname\",\"lifecycle_state\":\"pending\"}]}",
			muxVerify: func(t *testing.T, r *http.Request) {
				expectedValues := url.Values{"name": []string{"testname"}, "version": []string{models.APIVersion}}
				actualValues := r.URL.Query()
				assert.Equal(t, expectedValues, actualValues)
			},
			verify: func(t *testing.T, snapshots *models.SnapshotList, err error) {
				if assert.NotNil(t, snapshots) && assert.NotNil(t, snapshots.Snapshots) {
					assert.Equal(t, "testname", snapshots.Snapshots[0].ID)
				}
			},
		},
	}

	for _, testcase := range testCases {
		t.Run(testcase.name, func(t *testing.T) {
			emptyString := ""
			mux, client, teardown := test.SetupServer(t)
			test.SetupMuxResponse(t, mux, vpcfilevolume.Version+"/shares/r134-eb7f376a-29a3-4636-a97a-2ab4aa8ff663/snapshots", http.MethodGet, &emptyString, testcase.status, testcase.content, testcase.muxVerify)

			defer teardown()

			logger.Info("Test case being executed", zap.Reflect("testcase", testcase.name))

			snapshotService := vpcfilevolume.NewSnapshotManager(client)

			snapshots, err := snapshotService.ListSnapshots("r134-eb7f376a-29a3-4636-a97a-2ab4aa8ff663", testcase.limit, testcase.start, testcase.filters, logger)
			logger.Info("snapshots", zap.Reflect("snapshots", snapshots))

			if testcase.expectErr != "" && assert.Error(t, err) {
				assert.Equal(t, testcase.expectErr, err.Error())
				assert.Nil(t, snapshots)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, snapshots)
			}

			if testcase.verify != nil {
				testcase.verify(t, snapshots, err)
			}
		})
	}
}
