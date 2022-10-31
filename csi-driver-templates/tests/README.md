# Fix failing helm tests

To run Helm unit tests a helm plugin is needed:

```
helm plugin install https://github.com/quintush/helm-unittest 
```


* cd to root of the project `cd ../..`
* Run `helm unittest -3 ./csi-driver-templates` to run tests
  * Verify that failure is due to new changes that were added to the template files.
    If this the case, update template snapshot with `helm unittest -3 -u ./csi-driver-templates`
  * If templates failed due to other reasons, you should fix the test case/template files
