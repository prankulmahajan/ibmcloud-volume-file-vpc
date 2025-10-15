## How to execute E2E?

1. Create a VPC Cluster
2. Export the KUBECONFIG
   In kube config file use abosulte path for `certificate-authority`, `client-certificate` and `client-key`
3. Deploy the Driver (with SC)
4. Export enviornment variables
   ```
   # Mandatory
   export GO111MODULE=on
   export GOPATH=<GOPATH>
   export E2E_TEST_RESULT=<absolute-path to a file where the results should be redirected>
   export TEST_ENV=<stage/prod>
   export IC_REGION=<us-south>
   export IC_API_KEY_PROD=<prod API key> | export IC_API_KEY_STAG=<stage API key>
   export e2e_addon_version=<1.2 or 2.0>
   export icrImage=<Give the image which will be used by pods>
   export SC=<storage-class-name-with-delete-reclaim-policy>
   export SC_RETAIN=<storage-class-name-with-retain-reclaim-policy>

   # Optional
   export E2E_POD_COUNT="1"
   export E2E_PVC_COUNT="1"
   ```

5. Test DP2 profile with deployment
   ```
   ginkgo -v -nodes=1 --focus="\[ics-e2e\] \[sc\] \[with-deploy\]"  ./e2e
   ```
6. Test volume expansion
   ```
   ginkgo -v -nodes=1 --focus="\[ics-e2e\] \[resize\] \[pv\]"  ./e2e
   ```
7. Test EIT enabled volume test cases
   ```
   ginkgo -v -nodes=1 --focus="\[ics-e2e\] \[eit\]" ./e2e
   ```
   
8. Test RFS profile and it's storage classes
   ```
   ginkgo -v -nodes=1 --focus="\[ics-e2e\] \[sc_rfs\]"  ./e2e
   ```
