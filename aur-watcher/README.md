# aur-watcher

Checks the [Arch Linux User Repository](https://aur.archlinux.org/) for new package modifications and reports them to an endpoint.

To accomplish this [aur.archlinux.org/packages](https://aur.archlinux.org/packages/) is parsed. Package names and their version are stored in a CSV file. When a name-version-pair already exists in the CSV, we assume that we know all other updates.

This is an additional python script instead of a task in the [Golang controller](https://github.com/hashworks/aur-ci/controller) to enable quick adjustments in case the HTML changes.

## Alternative sources

There is an [RSS endpoint](https://aur.archlinux.org/rss/) which would be a better solution than the HTML frontend, but at the moment it is only for new packages. A merge request for package modifications [has been created](https://gitlab.archlinux.org/archlinux/aurweb/-/merge_requests/10).

Additionally, the AUR features an [RPC interface](https://aur.archlinux.org/rpc) which returns the modification timestamp, but only for given package names.