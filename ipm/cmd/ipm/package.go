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

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %v", err)
	}

	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return fmt.Errorf("private key is not an RSA key")
	}

	// Original-Tarball ohne Signatur laden
	tarball, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read package file: %v", err)
	}

	// Signatur erstellen
	hash := sha256.Sum256(tarball)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return fmt.Errorf("failed to sign package: %v", err)
	}

	// Temporäre Datei für neue .tgz mit Signatur
	tempFile, err := os.CreateTemp("", "signed-*.tgz")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name()) // Wird später überschrieben

	gw := gzip.NewWriter(tempFile)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Original-Tarball entpacken und kopieren
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("failed to open tarball: %v", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to read gzip: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tarball: %v", err)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}
		_, err = io.Copy(tw, tr)
		if err != nil {
			return fmt.Errorf("failed to copy file: %v", err)
		}
	}

	// Signatur als signature.sig hinzufügen
	sigHeader := &tar.Header{
		Name: "signature.sig",
		Mode: 0644,
		Size: int64(len(signature)),
	}
	if err := tw.WriteHeader(sigHeader); err != nil {
		return fmt.Errorf("failed to write signature header: %v", err)
	}
	if _, err := tw.Write(signature); err != nil {
		return fmt.Errorf("failed to write signature: %v", err)
	}

	// Tarball abschließen und umbenennen
	tw.Close()
	gw.Close()
	tempFile.Close()
	if err := os.Rename(tempFile.Name(), file); err != nil {
		return fmt.Errorf("failed to replace original tarball: %v", err)
	}

	return nil
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

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %v", err)
	}

	publicKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not an RSA key")
	}

	// Tarball öffnen
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("failed to open package file: %v", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to read gzip: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var signature []byte
	var tarballData []byte

	// Tarball ohne Signatur rekonstruieren und Signatur extrahieren
	tempFile, err := os.CreateTemp("", "unsigned-*.tgz")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	gw := gzip.NewWriter(tempFile)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tarball: %v", err)
		}
		if hdr.Name == "signature.sig" {
			signature, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("failed to read signature: %v", err)
			}
			continue
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}
		_, err = io.Copy(tw, tr)
		if err != nil {
			return fmt.Errorf("failed to copy file: %v", err)
		}
	}

	tw.Close()
	gw.Close()
	tempFile.Close()

	if signature == nil {
		log.Warn("Package is not signed", map[string]interface{}{
			"file": file,
		})
		return nil
	}

	// Unsigned Tarball laden
	tarballData, err = os.ReadFile(tempFile.Name())
	if err != nil {
		return fmt.Errorf("failed to read unsigned tarball: %v", err)
	}

	// Signatur verifizieren
	hash := sha256.Sum256(tarballData)
	err = rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signature)
	if err != nil {
		return fmt.Errorf("package signature verification failed: %v", err)
	}

	return nil
}