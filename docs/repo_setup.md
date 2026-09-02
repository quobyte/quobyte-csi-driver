## Additional repo setup

Please add internal repo to push target

```bash
git remote set-url --add --push origin ssh://<quobyte-internal-repo-url>/quobyte-csi-driver
git remote set-url --add --push origin git@github.com:quobyte/quobyte-csi-driver.git
```

The `git remote -v` output should look as following:

```bash
origin	git@github.com:quobyte/quobyte-csi-driver.git (fetch)
origin	ssh://<quobyte-internal-repo-url>/quobyte-csi-driver (push)
origin	git@github.com:quobyte/quobyte-csi-driver.git (push)
```