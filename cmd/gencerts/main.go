// TLS证书生成工具
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func main() {
	fmt.Println("正在生成自签名TLS证书...")

	// 确保目录存在
	certDir := "certs"
	if err := os.MkdirAll(certDir, 0755); err != nil {
		fmt.Printf("创建证书目录失败: %v\n", err)
		os.Exit(1)
	}

	// 生成私钥
	fmt.Println("生成RSA私钥...")
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Printf("生成私钥失败: %v\n", err)
		os.Exit(1)
	}

	// 创建证书模板
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Gatesvr Development"},
			Country:       []string{"CN"},
			Province:      []string{"Beijing"},
			Locality:      []string{"Beijing"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour), // 1年有效期
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:    []string{"localhost", "gatesvr", "*.gatesvr.local"},
	}

	// 生成证书
	fmt.Println("生成X.509证书...")
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		fmt.Printf("生成证书失败: %v\n", err)
		os.Exit(1)
	}

	// 保存证书文件
	certPath := filepath.Join(certDir, "server.crt")
	certOut, err := os.Create(certPath)
	if err != nil {
		fmt.Printf("创建证书文件失败: %v\n", err)
		os.Exit(1)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		fmt.Printf("写入证书文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("证书已保存到: %s\n", certPath)

	// 保存私钥文件
	keyPath := filepath.Join(certDir, "server.key")
	keyOut, err := os.Create(keyPath)
	if err != nil {
		fmt.Printf("创建私钥文件失败: %v\n", err)
		os.Exit(1)
	}
	defer keyOut.Close()

	privKeyDER, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		fmt.Printf("序列化私钥失败: %v\n", err)
		os.Exit(1)
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privKeyDER}); err != nil {
		fmt.Printf("写入私钥文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("私钥已保存到: %s\n", keyPath)

	// 验证生成的证书
	fmt.Println("验证证书...")
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		fmt.Printf("读取证书文件失败: %v\n", err)
		os.Exit(1)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		fmt.Println("解析证书PEM失败")
		os.Exit(1)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		fmt.Printf("解析证书失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n证书信息:")
	fmt.Printf("  主题: %s\n", cert.Subject)
	fmt.Printf("  有效期: %s 到 %s\n", cert.NotBefore.Format("2006-01-02 15:04:05"), cert.NotAfter.Format("2006-01-02 15:04:05"))
	fmt.Printf("  DNS名称: %v\n", cert.DNSNames)
	fmt.Printf("  IP地址: %v\n", cert.IPAddresses)

	fmt.Println("\n✅ TLS证书生成完成!")
	fmt.Println("现在可以启动QUIC服务器了。")
}
