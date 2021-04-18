package model

// TODO: Use controller model files

type Work struct {
	BuildId               int64
	PackageBase           string
	PackageBaseDataBase64 string
	Dependencies          []string
}
