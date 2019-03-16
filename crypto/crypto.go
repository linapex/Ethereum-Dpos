
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:36</date>
//</624342626555924480>


package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	secp256k1N, _  = new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	secp256k1halfN = new(big.Int).Div(secp256k1N, big.NewInt(2))
)

var errInvalidPubkey = errors.New("invalid secp256k1 public key")

//keccak256计算并返回输入数据的keccak256哈希。
func Keccak256(data ...[]byte) []byte {
	d := sha3.NewKeccak256()
	for _, b := range data {
		d.Write(b)
	}
	return d.Sum(nil)
}

//keccak256 hash计算并返回输入数据的keccak256哈希，
//将其转换为内部哈希数据结构。
func Keccak256Hash(data ...[]byte) (h common.Hash) {
	d := sha3.NewKeccak256()
	for _, b := range data {
		d.Write(b)
	}
	d.Sum(h[:0])
	return h
}

//keccak512计算并返回输入数据的keccak512哈希。
func Keccak512(data ...[]byte) []byte {
	d := sha3.NewKeccak512()
	for _, b := range data {
		d.Write(b)
	}
	return d.Sum(nil)
}

//createAddress创建一个给定字节和nonce的以太坊地址
func CreateAddress(b common.Address, nonce uint64) common.Address {
	data, _ := rlp.EncodeToBytes([]interface{}{b, nonce})
	return common.BytesToAddress(Keccak256(data)[12:])
}

//createAddress2根据地址字节创建以太坊地址，初始
//合同代码和盐。
func CreateAddress2(b common.Address, salt [32]byte, code []byte) common.Address {
	return common.BytesToAddress(Keccak256([]byte{0xff}, b.Bytes(), salt[:], Keccak256(code))[12:])
}

//toecdsa创建具有给定d值的私钥。
func ToECDSA(d []byte) (*ecdsa.PrivateKey, error) {
	return toECDSA(d, true)
}

//ToecdsSaunsafe盲目地将二进制blob转换为私钥。它应该差不多
//除非您确定输入有效并且希望避免敲击，否则不要使用。
//源代码编码错误（0个前缀被切断）。
func ToECDSAUnsafe(d []byte) *ecdsa.PrivateKey {
	priv, _ := toECDSA(d, false)
	return priv
}

//toecdsa创建具有给定d值的私钥。严格参数
//控制是否应在曲线大小或
//它还可以接受旧编码（0个前缀）。
func toECDSA(d []byte, strict bool) (*ecdsa.PrivateKey, error) {
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = S256()
	if strict && 8*len(d) != priv.Params().BitSize {
		return nil, fmt.Errorf("invalid length, need %d bits", priv.Params().BitSize)
	}
	priv.D = new(big.Int).SetBytes(d)

//私人d必须<n
	if priv.D.Cmp(secp256k1N) >= 0 {
		return nil, fmt.Errorf("invalid private key, >=N")
	}
//优先级d不能为零或负数。
	if priv.D.Sign() <= 0 {
		return nil, fmt.Errorf("invalid private key, zero or negative")
	}

	priv.PublicKey.X, priv.PublicKey.Y = priv.PublicKey.Curve.ScalarBaseMult(d)
	if priv.PublicKey.X == nil {
		return nil, errors.New("invalid private key")
	}
	return priv, nil
}

//FromECDSA将私钥导出到二进制转储。
func FromECDSA(priv *ecdsa.PrivateKey) []byte {
	if priv == nil {
		return nil
	}
	return math.PaddedBigBytes(priv.D, priv.Params().BitSize/8)
}

//UnmarshalSubkey将字节转换为secp256k1公钥。
func UnmarshalPubkey(pub []byte) (*ecdsa.PublicKey, error) {
	x, y := elliptic.Unmarshal(S256(), pub)
	if x == nil {
		return nil, errInvalidPubkey
	}
	return &ecdsa.PublicKey{Curve: S256(), X: x, Y: y}, nil
}

func FromECDSAPub(pub *ecdsa.PublicKey) []byte {
	if pub == nil || pub.X == nil || pub.Y == nil {
		return nil
	}
	return elliptic.Marshal(S256(), pub.X, pub.Y)
}

//hextoecdsa解析secp256k1私钥。
func HexToECDSA(hexkey string) (*ecdsa.PrivateKey, error) {
	b, err := hex.DecodeString(hexkey)
	if err != nil {
		return nil, errors.New("invalid hex string")
	}
	return ToECDSA(b)
}

//loadecdsa从给定文件加载secp256k1私钥。
func LoadECDSA(file string) (*ecdsa.PrivateKey, error) {
	buf := make([]byte, 64)
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	if _, err := io.ReadFull(fd, buf); err != nil {
		return nil, err
	}

	key, err := hex.DecodeString(string(buf))
	if err != nil {
		return nil, err
	}
	return ToECDSA(key)
}

//saveecdsa将secp256k1私钥保存到给定文件
//限制权限。密钥数据保存为十六进制编码。
func SaveECDSA(file string, key *ecdsa.PrivateKey) error {
	k := hex.EncodeToString(FromECDSA(key))
	return ioutil.WriteFile(file, []byte(k), 0600)
}

func GenerateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(S256(), rand.Reader)
}

//validateSignatureValues使用验证签名值是否有效
//给定的链规则。假设v值为0或1。
func ValidateSignatureValues(v byte, r, s *big.Int, homestead bool) bool {
	if r.Cmp(common.Big1) < 0 || s.Cmp(common.Big1) < 0 {
		return false
	}
//拒绝S值的上限（ECDSA延展性）
//参见secp256k1/libsecp256k1/include/secp256k1.h中的讨论。
	if homestead && s.Cmp(secp256k1halfN) > 0 {
		return false
	}
//前沿：允许S在全N范围内
	return r.Cmp(secp256k1N) < 0 && s.Cmp(secp256k1N) < 0 && (v == 0 || v == 1)
}

func PubkeyToAddress(p ecdsa.PublicKey) common.Address {
	pubBytes := FromECDSAPub(&p)
	return common.BytesToAddress(Keccak256(pubBytes[1:])[12:])
}

func zeroBytes(bytes []byte) {
	for i := range bytes {
		bytes[i] = 0
	}
}

