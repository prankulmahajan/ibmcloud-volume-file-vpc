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
	"strings"
	"testing"

	"github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/models"
	"github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/riaas/test"
	"github.com/IBM/ibmcloud-volume-file-vpc/common/vpcclient/vpcfilevolume"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestCreateSnapshot(t *testing.T) {
	// Setup new style zap logger
	logger, _ := GetTestContextLogger()
	defer logger.Sync()
	dummyURL := "/shares/r134-eb7f376a-29a3-4636-a97a-2ab4aa8ff663/snapshots"

	testCases := []struct {
		name string

		// backend url
		url string

		// Response
		status  int
		content string

		// Expected return
		expectErr string
		verify    func(*testing.T, *models.Snapshot, error)
	}{
		{
			name:   "Verify that the correct endpoint is invoked",
			status: http.StatusNoContent,
			url:    vpcfilevolume.Version + dummyURL,
		}, {
			name:      "Verify that a 500 is returned to the caller",
			status:    http.StatusInternalServerError,
			url:       vpcfilevolume.Version + dummyURL,
			content:   "{\"errors\":[{\"message\":\"testerr\",\"Code\":\"share_snapshot_creation_failed\"}], \"trace\":\"2af63776-4df7-4970-b52d-4e25676ec0e4\"}",
			expectErr: "Trace Code:2af63776-4df7-4970-b52d-4e25676ec0e4, Code:share_snapshot_creation_failed, Description:testerr, RC:500 Internal Server Error",
		}, {
			name:    "Verify that the snapshot is parsed correctly",
			status:  http.StatusOK,
			url:     vpcfilevolume.Version + dummyURL,
			content: "{\"id\":\"snapshot1\",\"lifecycle_state\":\"pending\"}",
			verify: func(t *testing.T, snapshot *models.Snapshot, err error) {
				assert.NotNil(t, snapshot)
				assert.Equal(t, snapshot.ID, "snapshot1")
			},
		},
	}

	for _, testcase := range testCases {
		t.Run(testcase.name, func(t *testing.T) {
			template := &models.Snapshot{
				Name: "snapshot-name",
				ID:   "snapshot-id",
			}
			mux, client, teardown := test.SetupServer(t)
			requestBody := `{
        			"id":"snapshot-id",
  			        "name":"snapshot-name"
      			}`
			requestBody = strings.Join(strings.Fields(requestBody), "") + "\n"
			test.SetupMuxResponse(t, mux, testcase.url, http.MethodPost, &requestBody, testcase.status, testcase.content, nil)

			defer teardown()

			logger.Info("Test case being executed", zap.Reflect("testcase", testcase.name))

			snapshotService := vpcfilevolume.NewSnapshotManager(client)

			snapshot, err := snapshotService.CreateSnapshot("r134-eb7f376a-29a3-4636-a97a-2ab4aa8ff663", template, logger)
			logger.Info("Snapshot", zap.Reflect("snapshot", snapshot))

			if testcase.expectErr != "" && assert.Error(t, err) {
				assert.Equal(t, testcase.expectErr, err.Error())
				assert.Nil(t, snapshot)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, snapshot)
			}

			if testcase.verify != nil {
				testcase.verify(t, snapshot, err)
			}
		})
	}
}
