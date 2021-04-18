package model

type Work struct {
	BuildId               int64
	PackageBase           string
	PackageBaseDataBase64 string
	Dependencies          []string
}
