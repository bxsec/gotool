package util

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
)

// Token 关于token 的方法
type Token struct {
}

//GetToken 生成token  name: 被加密的字符串   key: 加密的密钥  exptime:过期时间
func (c *Token) GetToken(name, key string, exptime time.Duration) string {

	//请求设置
	claims := make(jwt.MapClaims)
	claims["exp"] = time.Now().Add(exptime).Unix()
	claims["iat"] = time.Now().Unix()
	claims["name"] = name
	claims["User"] = "true"
	//新建一个请求  把请求设置写入其中 根据加密方式
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	//签名 字符串
	tokenString, _ := token.SignedString([]byte(key))

	return tokenString
}

//CheckToken 校验token是否有效 token: token  key:解密的密钥
func (c *Token) CheckToken(token, key string) (b bool, t *jwt.Token) {

	t, err := jwt.Parse(token, func(*jwt.Token) (interface{}, error) {
		return []byte(key), nil
	})
	if err != nil {

		return false, nil
	}
	return true, t
}

//CKToken 解析到期时间 key:密钥 返回到期时间和name 如果为0 token错误或过期
func (c *Token) CKToken(token, key string) (int64, string) {

	ok, u := c.CheckToken(token, key)
	if ok {
		if t, ok := u.Claims.(jwt.MapClaims); ok {
			na1 := t["exp"]
			name := t["name"]

			//把接口类型转成字符串
			str := fmt.Sprintf("%f", na1)
			name1 := fmt.Sprintf("%v", name)
			//把小数点后面的都去掉
			str = strings.Split(str, ".")[0]
			//转换为int64
			num, _ := strconv.Atoi(str)

			return int64(num), name1

		}
	}
	return 0, ""
}
