package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"ipm/pkg/log"
)

func initPackage(name string) error {
	pkg := map[string]interface{}{
		"name":    name,
		"version": "1.0.0",
		"main":    "index.js",
	}
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("package.json", data, 0644)
}

func packPackage(dir string) (string, error) {
	pkgFile := fmt.Sprintf("%s-1.0.0.tgz", filepath.Base(dir))
	out, err := os.Create(pkgFile)
	if err != nil {
		return "", err
	}
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(path[len(dir)+1:])
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return "", err
	}
	return pkgFile, nil
}

func signPackage(file, keyFile string) error {
	if keyFile == "" {
		return fmt.Errorf("private key file required (--key)")
	}

	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("failed to read private key: %v", err)
	}
	block, _ := pem.Decode(keyData)
	if block == nil {
		return fmt.Errorf("invalid private key format")
	}

	// Verwende ParsePKCS8PrivateKey statt ParsePKCS1PrivateKey
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %v", err)
	}

	// Typumwandlung zu *rsa.PrivateKey
	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return fmt.Errorf("private key is not an RSA key")
	}

	tarball, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read package file: %v", err)
	}

	hash := sha256.Sum256(tarball)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return fmt.Errorf("failed to sign package: %v", err)
	}

	sigFile := file + ".sig"
	return os.WriteFile(sigFile, signature, 0644)
}

func verifyPackage(file, pubKeyFile string) error {
	if pubKeyFile == "" {
		return fmt.Errorf("public key file required (--pubkey)")
	}

	pubKeyData, err := os.ReadFile(pubKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read public key: %v", err)
	}
	block, _ := pem.Decode(pubKeyData)
	if block == nil {
		return fmt.Errorf("invalid public key format")
	}

	// Verwende ParsePKIXPublicKey statt ParsePKCS1PublicKey
	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %v", err)
	}

	// Typumwandlung zu *rsa.PublicKey
	publicKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not an RSA key")
	}

	tarball, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read package file: %v", err)
	}

	sigFile := file + ".sig"
	signature, err := os.ReadFile(sigFile)
	if os.IsNotExist(err) {
		log.Warn("Package is not signed", map[string]interface{}{
			"file": file,
		})
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to read signature file: %v", err)
	}

	hash := sha256.Sum256(tarball)
	err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signature)
	if err != nil {
		log.Warn("Package signature verification failed", map[string]interface{}{
			"file":  file,
			"error": err,
		})
		return nil
	}
	return nil
}