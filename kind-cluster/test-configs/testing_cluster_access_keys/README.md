# Quobyte General purpose access keys test

## Requirements

* Your cluster should have general purpose access key used in k8s secret included in this test
* You can import the access key(s) with (requires base64 decoding) values from the secret and
  providing data as csv via qmgmt (`qmggmt -u $API_URL accesskey import <access_key>.csv`). The
  access should have access to the tenant specified in the storage class.
