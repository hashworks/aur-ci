package aur

import (
	"bufio"
	"net/http"

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

func GetPackageBases() ([]string, error) {
	resp, err := http.Get("https://aur.archlinux.org/pkgbase.gz")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	pkgbases := []string{}
	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}
		pkgbases = append(pkgbases, scanner.Text())
	}

	return pkgbases, nil
}
