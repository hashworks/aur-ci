# Continuous Integration System for the Arch Linux User Repository (AUR)

WIP. Basic idea is to provide a system that automatically creates virtual machines to build new AUR packages/modifications and report back the result / log. Created packages won't be provided to the public.

This project was created as a Master Project for my studies at the [Leipzig University of Applied Sciences](https://www.htwk-leipzig.de/en/htwk-leipzig/).

## aur-watcher

Monitors the AUR for package modifications and reports them to the controller.

## controller

Retrieves package information from the AUR after reports from the aur-watcher, including a full git commit log. The latest commit is added to a build queue.

If the build queue size exceeds a certain threshold one or multiple VMs are created as workers. To accomplish this the [Hetzner Cloud API](https://docs.hetzner.cloud/) is used. If VMs are no longer needed the controller will remove them as well.

In the future the controller will also provide a web frontend and send out notifications about failed builds to maintainers.

## worker

Workers register themselves at the controller and request a work task from the package build queue. The package is build in a Docker container using the [`archlinux/archlinux:base-devel`](https://hub.docker.com/_/archlinux) Image. The result / build log is send back to the controller.