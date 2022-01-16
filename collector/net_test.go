// Copyright 2021 MicroOps
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"bufio"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/MicroOps-cn/data_exporter/testings"
	"github.com/prometheus/common/config"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"math/big"
	"math/rand"
	"net"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

var exampleData = `
{"data":{"server1":{"metrics":{"CPU":"16","Memory":68719476736}},"server2":{"metrics":{"CPU":"8","Memory":34359738368}}},"code":0}
{"data":{"server3":{"metrics":{"CPU":"16","Memory":68719476736}},"server4":{"metrics":{"CPU":"8","Memory":34359738368}}},"code":0}
-----------//---------------
Hello Go

`

type connect interface {
	io.ReadWriteCloser
}

func handlerConn(t *testings.T, conn connect, connId string) {
	defer conn.Close()
	buf := bufio.NewScanner(conn)
	for buf.Scan() {
		if buf.Text() == "get_data" {
			for _, s := range strings.Split(exampleData, "\n") {
				t.Logf("[%s]返回监控数据: %s\n", connId, s)
				_, err := conn.Write([]byte(fmt.Sprintf("%s\n", s)))
				if err != nil {
					break
				}
				time.Sleep(time.Second / 2)
			}
		}
	}
	t.Logf("[%s]连接已关闭", connId)
}

func createX509KeyPair(tt *testings.T) (string, string) {
	tmpDir := path.Join(os.TempDir(), fmt.Sprintf("test-data-%d", rand.Int63()))
	tt.AssertNoError(os.MkdirAll(tmpDir, 0755))
	certPath := path.Join(tmpDir, "cert.pem")
	keyPath := path.Join(tmpDir, "key.pem")

	tt.Logf("create x509 key-pair: %s::%s", certPath, keyPath)
	max := new(big.Int).Lsh(big.NewInt(1), 128)       //把 1 左移 128 位，返回给 big.Int
	serialNumber, err := crand.Int(crand.Reader, max) //返回在 [0, max) 区间均匀随机分布的一个随机值
	tt.AssertNoError(err)
	subject := pkix.Name{ //Name代表一个X.509识别名。只包含识别名的公共属性，额外的属性被忽略。
		Organization:       []string{"Manning Publications Co."},
		OrganizationalUnit: []string{"Books"},
		CommonName:         "Go Web Programming",
	}
	template := x509.Certificate{
		SerialNumber: serialNumber, // SerialNumber 是 CA 颁布的唯一序列号，在此使用一个大随机数来代表它
		Subject:      subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature, //KeyUsage 与 ExtKeyUsage 用来表明该证书是用来做服务器认证的
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},               // 密钥扩展用途的序列
		IPAddresses:  []net.IP{net.ParseIP("127.0.1.1")},
	}
	pk, err := rsa.GenerateKey(crand.Reader, 2048) //生成一对具有指定字位数的RSA密钥
	tt.AssertNoError(err)
	//CreateCertificate基于模板创建一个新的证书
	//第二个第三个参数相同，则证书是自签名的
	//返回的切片是DER编码的证书
	derBytes, err := x509.CreateCertificate(crand.Reader, &template, &template, &pk.PublicKey, pk) //DER 格式
	tt.AssertNoError(err)
	certOut, err := os.Create(certPath)
	tt.AssertNoError(err)
	defer certOut.Close()
	tt.AssertNoError(pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}))
	keyOut, err := os.Create(keyPath)
	tt.AssertNoError(err)
	defer keyOut.Close()
	tt.AssertNoError(pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(pk)}))
	return certPath, keyPath
}

func runServ(t *testings.T, protocol, addr string, tlsConfig *tls.Config) io.Closer {
	var err error
	if protocol == "tcp" {
		var listen net.Listener
		if tlsConfig == nil {
			t.Logf("listen: tcp://%s", addr)
			listen, err = net.Listen("tcp", addr)
			t.AssertNoError(err)
		} else {
			t.Logf("listen: tcps://%s", addr)
			listen, err = tls.Listen("tcp", addr, tlsConfig)
			t.AssertNoError(err)
		}
		go func() {
			for {
				conn, err := listen.Accept()
				if err != nil && assert.Contains(t.T, err.Error(), "use of closed network connection") {
					break
				}
				t.Logf("[S]连接已建立, %s", conn.RemoteAddr())
				go handlerConn(t, conn, conn.RemoteAddr().String())
			}
			t.Logf("[S]listen closed %s", addr)
		}()
		return listen
	} else if protocol == "udp" {
		if tlsConfig != nil {
			t.Errorf("unsupport protocol: tls over udp")
		} else {
			t.Logf("listen: udp://%s", addr)
			udpAddr, err := net.ResolveUDPAddr("udp", addr)
			t.AssertNoError(err)
			conn, err := net.ListenUDP("udp", udpAddr)
			t.AssertNoError(err)

			go func() {
				defer conn.Close()
				for {
					var data [4096]byte
					n, addr, err := conn.ReadFromUDP(data[:]) // 接收数据
					if err != nil && assert.Contains(t.T, err.Error(), "use of closed network connection") {
						break
					}
					if strings.TrimSpace(string(data[:n])) == "get_data" {
						for _, s := range strings.Split(exampleData, "\n") {
							t.Logf("[%s]返回监控数据: %s\n", addr, s)
							_, err := conn.WriteToUDP([]byte(fmt.Sprintf("%s\n", s)), addr)
							if err != nil {
								break
							}
							time.Sleep(time.Second)
						}
					}
				}
				t.Logf("[S]listen closed %s", addr)
			}()
			return conn
		}
	}
	t.AssertNoError(fmt.Errorf("未知的协议配置:: protocol: %s, tls: %v", protocol, tlsConfig))
	return nil
}

func TestTCPNetConfig(t *testing.T) {
	testNetTlsConfig(t, "tcp", runServ)
	testNetConfig(t, "tcp", runServ)
}
func TestUDPNetConfig(t *testing.T) {
	testNetConfig(t, "udp", runServ)
}

func newInt64(v int64) *int64 {
	return &v
}

func testNetTlsConfig(t *testing.T, network string, listenTls func(*testings.T, string, string, *tls.Config) io.Closer) {
	rand.Seed(time.Now().UTC().UnixNano())
	tt := testings.NewTesting(t)
	certPath, keyPath := createX509KeyPair(tt)
	defer os.Remove(certPath)
	defer os.Remove(keyPath)
	certPem, err := ioutil.ReadFile(certPath)
	tt.AssertNoError(err)
	keyPem, err := ioutil.ReadFile(keyPath)
	tt.AssertNoError(err)
	cert, err := tls.X509KeyPair(certPem, keyPem)
	tt.AssertNoError(err)

	addr := fmt.Sprintf("127.0.1.1:%d", rand.Intn(50000)+15530)
	tt.Logf("start %s listen serv: %s", network, addr)
	listen := listenTls(tt, network, addr, &tls.Config{Certificates: []tls.Certificate{cert}})
	tt.AssertNoError(err)
	defer func() {
		time.Sleep(time.Second)
		tt.Logf("stop %s listen serv: %s", network, addr)
		listen.Close()
	}()
	ds := Datasource{
		Name:     fmt.Sprintf("Test %s Datasource", strings.ToUpper(network)),
		Url:      addr,
		Type:     DatasourceType(network),
		Timeout:  time.Second * 30,
		ReadMode: StreamLine,
		Config: &NetConfig{
			Send: SendConfigs{SendConfig{
				Msg: "get_data\n", Delay: 0,
			}},
			protocol:        network,
			MaxConnectTime:  time.Second,
			MaxTransferTime: (*time.Duration)(newInt64(int64(time.Second * 7))),
			TLSConfig: &config.TLSConfig{
				CAFile: certPath,
			},
			EndOf: "//",
		},
		MaxContentLength: newInt64(4096),
	}

	func() {
		t.Log("测试ReadLine: ")
		stream, err := ds.GetLineStream(tt.Context)
		tt.AssertNoError(err)
		defer stream.Close()
		for {
			line, err := stream.ReadLine()
			if err == io.EOF {
				break
			}
			tt.AssertNoError(err)
			tt.Log("收到数据: ", string(line))
		}
	}()

	func() {
		t.Log("测试ReadAll: ")

		all, err := ds.ReadAll(tt.Context)
		tt.AssertNoError(err)
		t.Log("收到数据: ", string(all))
	}()
}
func testNetConfig(t *testing.T, network string, listenFunc func(*testings.T, string, string, *tls.Config) io.Closer) {
	rand.Seed(time.Now().UTC().UnixNano())
	tt := testings.NewTesting(t)

	addr := fmt.Sprintf("127.0.1.1:%d", rand.Intn(50000)+15530)
	tt.Logf("start %s listen serv: %s", network, addr)
	listen := listenFunc(tt, network, addr, nil)
	defer func() {
		time.Sleep(time.Second)
		tt.Logf("stop %s listen serv: %s", network, addr)
		listen.Close()
	}()
	ds := Datasource{
		Name:     fmt.Sprintf("Test %s Datasource", strings.ToUpper(network)),
		Url:      addr,
		Type:     DatasourceType(network),
		Timeout:  time.Second * 30,
		ReadMode: StreamLine,
		TCPConfig: &NetConfig{
			Send: SendConfigs{SendConfig{
				Msg: "get_data\n", Delay: 0,
			}},
			protocol:        network,
			MaxConnectTime:  time.Second,
			MaxTransferTime: (*time.Duration)(newInt64(int64(time.Second * 5))),
			EndOf:           "//",
		},
		MaxContentLength: newInt64(4096),
	}
	ds.UDPConfig = ds.TCPConfig
	ds.Config = ds.TCPConfig
	func() {
		t.Log("测试ReadLine: ")
		stream, err := ds.GetLineStream(tt.Context)
		tt.AssertNoError(err)
		defer stream.Close()
		for {
			line, err := stream.ReadLine()
			if err == io.EOF {
				break
			}
			tt.AssertNoError(err)
			tt.Log("收到数据: ", string(line))
		}
	}()
	func() {
		t.Log("测试ReadAll: ")
		all, err := ds.ReadAll(tt.Context)
		tt.AssertNoError(err)
		t.Log("收到数据: ", string(all))
	}()
}
