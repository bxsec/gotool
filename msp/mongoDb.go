package msp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//MongoDB 数据库常用方法
type MongoDB struct {
	Client *mongo.Client      //连接
	Ctx    context.Context    //环境
	Cancel context.CancelFunc //关闭数据库连接
}

//SetDB 初始化数据库 前置条件 需要设置数据库 url
func (c *MongoDB) SetDB(url string) (err error) {

	client, err := mongo.NewClient(options.Client().ApplyURI(url))
	if err != nil {
		fmt.Println("数据库连接失败")
		return errors.New("数据库连接失败")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	c.Client = client
	c.Ctx = ctx
	c.Cancel = cancel

	err = client.Connect(ctx)
	if err != nil {
		return err
	}
	return nil
}
