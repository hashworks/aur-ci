package model

import (
	"time"

	rpc "github.com/mikkeloscar/aur"
)

type Package struct {
	//Id             int64     `json:"ID"`
	Name           string    `json:"Name" xorm:"pk notnull"`
	PackageBaseId  int64     `json:"PackageBaseID" xorm:"index notnull"`
	PackageBase    string    `json:"PackageBase" xorm:"index notnull"`
	Version        string    `json:"Version" xorm:"notnull"`
	Description    string    `json:"Description"`
	URL            string    `json:"URL" xorm:"'url'"`
	NumVotes       int       `json:"NumVotes"`
	Popularity     float64   `json:"Popularity"`
	OutOfDate      time.Time `json:"OutOfDate"`
	Maintainer     string    `json:"Maintainer"`
	FirstSubmitted time.Time `json:"FirstSubmitted"`
	LastModified   time.Time `json:"LastModified"`
	URLPath        string    `json:"URLPath" xorm:"'url_path'"`
	Depends        []string  `json:"Depends"`
	MakeDepends    []string  `json:"MakeDepends"`
	CheckDepends   []string  `json:"CheckDepends"`
	Conflicts      []string  `json:"Conflicts"`
	Provides       []string  `json:"Provides"`
	Replaces       []string  `json:"Replaces"`
	OptDepends     []string  `json:"OptDepends"`
	Groups         []string  `json:"Groups"`
	License        []string  `json:"License"`
	Keywords       []string  `json:"Keywords"`
}

func NewPackageFromRPCPackage(pkg rpc.Pkg) Package {
	return Package{
		//Id:             int64(pkg.ID), // The ID appears to be volatile and serves us no purpose.
		Name:           pkg.Name,
		PackageBaseId:  int64(pkg.PackageBaseID),
		PackageBase:    pkg.PackageBase,
		Version:        pkg.Version,
		Description:    pkg.Description,
		URL:            pkg.URL,
		NumVotes:       pkg.NumVotes,
		Popularity:     pkg.Popularity,
		OutOfDate:      time.Unix(int64(pkg.OutOfDate), 0),
		Maintainer:     pkg.Maintainer,
		FirstSubmitted: time.Unix(int64(pkg.FirstSubmitted), 0),
		LastModified:   time.Unix(int64(pkg.LastModified), 0),
		URLPath:        pkg.URLPath,
		Depends:        pkg.Depends,
		MakeDepends:    pkg.MakeDepends,
		CheckDepends:   pkg.CheckDepends,
		Conflicts:      pkg.Conflicts,
		Provides:       pkg.Provides,
		Replaces:       pkg.Replaces,
		OptDepends:     pkg.OptDepends,
		Groups:         pkg.Groups,
		License:        pkg.License,
		Keywords:       pkg.Keywords,
	}
}
