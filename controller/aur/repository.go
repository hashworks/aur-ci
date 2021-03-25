package aur

import (
	"github.com/hashworks/aur-ci/controller/model"
	rpc "github.com/mikkeloscar/aur"
)

func GetPackageInfos(packageNames []string) ([]model.Package, error) {
	var packages []model.Package

	rpcPackages, err := rpc.Info(packageNames)
	if err != nil {
		return packages, err
	}

	for _, rpcPackage := range rpcPackages {
		packages = append(packages, model.NewPackageFromRPCPackage(rpcPackage))
	}

	return packages, nil
}
