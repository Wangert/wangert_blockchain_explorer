package main

import (
	"flag"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/common"
	"html/template"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"path/filepath"
	"github.com/syndtr/goleveldb/leveldb"
	"encoding/binary"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"bytes"
	"time"
	"os"
	"strings"
	"strconv"
)

//最近的区块号
const NUMBER = 102
//const NUMBER = 8767

//链ID
const CHAINID = 1314
//const CHAINID = 123

//区块levelDB文件路径
const DBPATH = "./chaindata/"
//const DBPATH = "./chaindatatest/"

//区块链信息结构体
type BlockchainInfo struct {
	LastBlockNum *big.Int
	ThisBlockNum *big.Int
	Blocks       []BlockInfo
}

//区块信息结构体
type BlockInfo struct {
	//区块高度
	Num              *big.Int
	//时间戳
	Timestamp        time.Time
	//区块Hash
	Hash             string
	//交易数量
	TransactionCount int
	//矿工
	Miner            string
}

//交易数组
type TransactionArray struct {
	//该交易所属区块的高度
	Number *big.Int
	//该交易所属区块的Hash
	BlockHash string
	//交易数组
	TransactionsInfo []TransactonInfo
}


//交易信息
type TransactonInfo struct {
	//Index
	TxIndex uint64
	//交易Hash
	TxHash	string
	//交易发送者
	From	string
	//交易接收者
	To		string
	//交易金额
	Value   *big.Int

}

//区块时间戳与交易数结构体
type DataChart struct {
	//时间戳(横轴)
	Timestamps []time.Time
	//交易数(纵轴)
	TransactionCounts []int
	//有交易的用户账户列表
	AddressList []string
	//用户作为交易发送者次数
	SenderCounts []int
	//用户作为交易接受者次数
	RecipientCounts []int
	//用户总收款金额
	TotalValue []*big.Int
}

type OptionsStruct struct {
	Host          string
	Port          int
	WWWRoot       string
	TemplatesGlob string
	EthEndpoint   string
}

var Options OptionsStruct
var Templates *template.Template
var Client *ethclient.Client
var RPCClient *rpc.Client
var MaxBlockNum int64


var (
	headerPrefix = []byte("h")
	headerHashSuffix = []byte("n")
	bodyPrefix = []byte("b")
)

//获取区块链信息
func GetBlockchainInfo() *BlockchainInfo {
	blockchainInfo := &BlockchainInfo{}


	//最新的区块高度
	number := uint64(NUMBER)

	//打开数据库文件
	db, err := leveldb.OpenFile(DBPATH, nil)

	defer db.Close()

	if err != nil {
		log.Panic(err)
	}


	//需要显示的区块数量
	//maxBlock := 30

	for {

		//获取区块头hash
		data, _ := db.Get(headerHashKey(number), nil)
		//字节数组转hash
		headerHash := common.BytesToHash(data)

		if headerHash == (common.Hash{}) {
			fmt.Println("Not find this headerHash!")
			os.Exit(1)
		}

		//fmt.Println(headerHash)

		//根据区块头Hash获取区块
		block := getBlock(db, headerHash, number)

		//设置区块信息
		blockInfo := &BlockInfo{
			block.Number(),
			time.Unix(block.Time().Int64(), 0),
			headerHash.String(),
			block.Transactions().Len(),
			block.Coinbase().String(),
		}

		blockchainInfo.LastBlockNum = new(big.Int).SetInt64(NUMBER)
		blockchainInfo.ThisBlockNum = block.Number()
		blockchainInfo.Blocks = append(blockchainInfo.Blocks, *blockInfo)

		if number == 1 {
			break
		}

		//前一个区块的高度
		number = number - 1

		}

	return blockchainInfo
}

//根据区块链高度获取transactions
func GetTransactionArrayByBlockNumber(number uint64) *TransactionArray {

	transactionArray := &TransactionArray{}

	//打开数据库文件
	db, err := leveldb.OpenFile(DBPATH, nil)

	defer db.Close()

	if err != nil {
		log.Panic(err)
	}

	//获取区块头hash
	data, _ := db.Get(headerHashKey(number), nil)
	//字节数组转hash
	headerHash := common.BytesToHash(data)

	if headerHash == (common.Hash{}) {
		fmt.Println("Not find this headerHash!")
		os.Exit(1)
	}

	//fmt.Println(headerHash)

	//根据区块头Hash获取区块
	block := getBlock(db, headerHash, number)
	//设置该交易组所属区块的高度
	transactionArray.Number = new(big.Int).SetUint64(number)
	//设置该交易组所属区块的Hash
	transactionArray.BlockHash = headerHash.String()

	//设置chainID
	s := types.NewEIP155Signer(new(big.Int).SetInt64(CHAINID))

	//若没有交易则直接返回空交易数组
	if block.Transactions().Len() != 0 {
		for index, tx := range block.Transactions() {

			address, _ := s.Sender(tx)

			transactionInfo := &TransactonInfo{
				uint64(index),
				tx.Hash().String(),
				address.String(),
				tx.To().String(),
				tx.Value(),
			}

			transactionArray.TransactionsInfo = append(transactionArray.TransactionsInfo, *transactionInfo)

		}

		return transactionArray
	}

	return transactionArray

}

//统计区块时间戳和区块交易数
func GetChartData() *DataChart {

	dataChart := &DataChart{}

	//最新的区块高度
	number := uint64(NUMBER)

	//打开数据库文件
	db, err := leveldb.OpenFile(DBPATH, nil)

	defer db.Close()

	if err != nil {
		log.Panic(err)
	}



	dataChart.AddressList = GetAddressList(db, number)
	dataChart.SenderCounts, dataChart.RecipientCounts = GetSenderAndRecipientCountsByAddress(db, number, dataChart.AddressList)
	dataChart.TotalValue = GetAddressTotalValue(db, number, dataChart.AddressList)

	//需要显示的区块数量
	//maxBlock := 30

	for {

		//获取区块头hash字节数组
		data, _ := db.Get(headerHashKey(number), nil)
		//字节数组转hash
		headerHash := common.BytesToHash(data)

		if headerHash == (common.Hash{}) {
			fmt.Println("Not find this headerHash!")
			os.Exit(1)
		}

		//fmt.Println(headerHash)

		//根据区块头Hash获取区块
		block := getBlock(db, headerHash, number)

		dataChart.Timestamps = append(dataChart.Timestamps, time.Unix(block.Time().Int64(), 0))
		dataChart.TransactionCounts = append(dataChart.TransactionCounts, block.Transactions().Len())

		if number == 1 {
			dataChart.Timestamps = reverse(dataChart.Timestamps).([]time.Time)
			dataChart.TransactionCounts = reverse(dataChart.TransactionCounts).([]int)
			break
		}

		number = number - 1

	}



	return dataChart
}

//获取所有交易发起者的address
func GetAddressList(db *leveldb.DB, number uint64) []string {

	var addressList []string

	for {
		//获取区块头hash字节数组
		data, _ := db.Get(headerHashKey(number), nil)
		//字节数组转hash
		headerHash := common.BytesToHash(data)

		if headerHash == (common.Hash{}) {
			fmt.Println("Not find this headerHash!")
			os.Exit(1)
		}

		//根据区块头Hash获取区块
		block := getBlock(db, headerHash, number)

		//设置chainID
		s := types.NewEIP155Signer(new(big.Int).SetInt64(CHAINID))

		//若没有交易则直接返回空交易数组
		if block.Transactions().Len() != 0 {
			for _, tx := range block.Transactions() {
				//获取该交易发送者的Address
				senderAddress, _ := s.Sender(tx)
				//获取该交易接受者的Address
				recipientAddress := tx.To()

				senderAddressString := senderAddress.String()

				//0表示不存在
				flagSender := 0
				flagRecipient := 0

				var recipientAddressString string

				if recipientAddress != nil {
					recipientAddressString = recipientAddress.String()
					flagRecipient = 1
				}

				for _, addr := range addressList {

					//若地址已存在将flag置1
					if senderAddressString == addr {
						flagSender = 1
					}

					if flagRecipient != 1 && recipientAddressString == addr {
						flagRecipient = 1
					}

				}

				//若不存在添加到切片中
				if flagSender == 0 {
					addressList = append(addressList, senderAddressString)
				}

				if flagRecipient == 0 {
					addressList = append(addressList, recipientAddressString)
				}

			}
		}

		if number == 1 {
			break
		}

		number = number - 1

	}

	//fmt.Println("addressList:", addressList)

	return addressList
}

//根据地址查询其交易作为交易发送者的次数
func GetSenderAndRecipientCountsByAddress(db *leveldb.DB, number uint64, addressList []string) ([]int, []int) {

	//存储地址作为发送者次数
	var senderCounts []int
	//村塾地址作为接受者次数
	var recipientCounts []int

	for range addressList {
		senderCounts = append(senderCounts, 0)
		recipientCounts = append(recipientCounts, 0)
	}

	for {
		//获取区块头hash字节数组
		data, _ := db.Get(headerHashKey(number), nil)
		//字节数组转hash
		headerHash := common.BytesToHash(data)

		if headerHash == (common.Hash{}) {
			fmt.Println("Not find this headerHash!")
			os.Exit(1)
		}

		//根据区块头Hash获取区块
		block := getBlock(db, headerHash, number)

		//设置chainID
		s := types.NewEIP155Signer(new(big.Int).SetInt64(CHAINID))

		//若没有交易则直接返回空交易数组
		if block.Transactions().Len() != 0 {
			for index, address := range addressList {

				for _, tx := range block.Transactions() {
					//获取该交易发送者的Address
					senderAddress, _ := s.Sender(tx)
					//获取该交易接受者的Address
					recipientAddress := tx.To()

					senderAddressString := senderAddress.String()

					var recipientAddressString string
					if recipientAddress != nil {
						recipientAddressString = recipientAddress.String()
					}

					if address == senderAddressString {
						senderCounts[index] = senderCounts[index] + 1
						continue
					}

					if address == recipientAddressString {
						recipientCounts[index] = recipientCounts[index] + 1
					}

				}

			}

		}

		if number == 1 {
			break
		}

		number = number - 1
	}

	return senderCounts, recipientCounts

}

//获取用户收款总金额
func GetAddressTotalValue(db *leveldb.DB, number uint64, addressList []string) []*big.Int {

	var totalValue []*big.Int

	for range addressList {
		totalValue = append(totalValue, new(big.Int).SetUint64(0))
	}

	for {
		//获取区块头hash字节数组
		data, _ := db.Get(headerHashKey(number), nil)
		//字节数组转hash
		headerHash := common.BytesToHash(data)

		if headerHash == (common.Hash{}) {
			fmt.Println("Not find this headerHash!")
			os.Exit(1)
		}

		//根据区块头Hash获取区块
		block := getBlock(db, headerHash, number)

		//若没有交易则直接返回空交易数组
		if block.Transactions().Len() != 0 {
			for index, address := range addressList {

				for _, tx := range block.Transactions() {

					//获取该交易接受者的Address
					recipientAddress := tx.To()

					var recipientAddressString string
					if recipientAddress != nil {
						recipientAddressString = recipientAddress.String()
					}

					if address == recipientAddressString {
						totalValue[index].Add(totalValue[index], tx.Value())
					}

				}

			}

		}

		if number == 1 {
			break
		}

		number = number - 1
	}

	return totalValue
}

//切片反转
func reverse(p interface{}) interface{} {
	switch p.(type) {
	case []time.Time:
		s, _ := p.([]time.Time)
		for i, j := 0, len(s) - 1; i < j; i, j = i + 1, j - 1 {
			s[i], s[j] = s[j], s[i]
		}

		return s

	case []int:
		s, _ := p.([]int)
		for i, j := 0, len(s) - 1; i < j; i, j = i + 1, j - 1 {
			s[i], s[j] = s[j], s[i]
		}

		return s
	}

	return nil
}

//省略超长hex
func ShortHex(long string) string {
	if len(long) < 19 {
		return long
	}

	return long[0:8] + "..." + long[len(long)-8:]
}

func HandleTemplates(next http.Handler) http.Handler {
	//templatedExtension := ".html"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request from %v: %v", r.RemoteAddr, r.URL.Path)

		//fmt.Println("test:", r.URL.RawQuery)

		if r.URL.Path == "/index.html" || r.URL.Path == "/" {
			//获取区块链信息
			blockchainInfo := GetBlockchainInfo()

			if blockchainInfo == nil {
				log.Println("Unable to retrieve blockchain info")
				return
			}

			if err := r.ParseForm(); err != nil {
				log.Println("Error parsing form parameters")
			}


			diskFilename := filepath.Base(r.URL.Path)
			if r.URL.Path == "/" {
				diskFilename = "index.html"
			}


			diskFilepath := filepath.Join(Options.WWWRoot, diskFilename)

			newTemplates, err := Templates.Clone()
			if err != nil {
				log.Printf("Unable to clone templates: %v", err.Error())
				return
			}

			templateData, err := ioutil.ReadFile(diskFilepath)
			if err != nil {
				log.Printf("Unable to read the requested file: %v", err.Error())
				return
			}

			funcMap := template.FuncMap{
				"ShortHex": ShortHex,
			}

			newTemplates, err = newTemplates.New("main").Funcs(funcMap).Parse(string(templateData))
			if err != nil {
				log.Printf("Unable to parse template for requested file: %v", err.Error())
				return
			}

			newTemplates.Execute(w, blockchainInfo)
			if err != nil {
				log.Printf("Unable to execute template for requested file: %v", err.Error())
				return
			}
			return

		} else if r.URL.Path == "/blocktransactions.html" {

			s := strings.Split(r.URL.RawQuery, "=")

			number, _ := strconv.ParseUint(s[1], 10, 64)

			//fmt.Println(number)

			transactionArray := GetTransactionArrayByBlockNumber(number)

			//fmt.Println(transactionArray)

			if transactionArray == nil {
				log.Println("Unable to retrieve transaction info")
				return
			}

			if err := r.ParseForm(); err != nil {
				log.Println("Error parsing form parameters")
			}


			diskFilename := filepath.Base(r.URL.Path)

			diskFilepath := filepath.Join(Options.WWWRoot, diskFilename)

			fmt.Println(diskFilepath)

			newTemplates, err := Templates.Clone()
			if err != nil {
				log.Printf("Unable to clone templates: %v", err.Error())
				return
			}

			templateData, err := ioutil.ReadFile(diskFilepath)
			if err != nil {
				log.Printf("Unable to read the requested file: %v", err.Error())
				return
			}

			funcMap := template.FuncMap{
				"ShortHex": ShortHex,
			}

			newTemplates, err = newTemplates.New("main").Funcs(funcMap).Parse(string(templateData))
			if err != nil {
				log.Printf("Unable to parse template for requested file: %v", err.Error())
				return
			}

			newTemplates.Execute(w, transactionArray)
			if err != nil {
				log.Printf("Unable to execute template for requested file: %v", err.Error())
				return
			}
			return

		} else if r.URL.Path == "/datachart.html" {

			dataChart := GetChartData()


			if dataChart == nil {
				log.Println("Unable to retrieve transaction info")
				return
			}

			if err := r.ParseForm(); err != nil {
				log.Println("Error parsing form parameters")
			}


			diskFilename := filepath.Base(r.URL.Path)

			diskFilepath := filepath.Join(Options.WWWRoot, diskFilename)

			fmt.Println(diskFilepath)

			newTemplates, err := Templates.Clone()
			if err != nil {
				log.Printf("Unable to clone templates: %v", err.Error())
				return
			}

			templateData, err := ioutil.ReadFile(diskFilepath)
			if err != nil {
				log.Printf("Unable to read the requested file: %v", err.Error())
				return
			}

			funcMap := template.FuncMap{
				"ShortHex": ShortHex,
			}

			newTemplates, err = newTemplates.New("main").Funcs(funcMap).Parse(string(templateData))
			if err != nil {
				log.Printf("Unable to parse template for requested file: %v", err.Error())
				return
			}

			newTemplates.Execute(w, dataChart)
			if err != nil {
				log.Printf("Unable to execute template for requested file: %v", err.Error())
				return
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}

//初始化模板
func InitTemplates() {
	templateFiles, err := filepath.Glob(Options.TemplatesGlob)
	if err != nil {
		log.Fatal(err)
	}

	Templates = template.Must(template.New("base").Parse(""))

	for _, templateFile := range templateFiles {
		templateData, err := ioutil.ReadFile(templateFile)
		if err != nil {
			log.Fatal(err)
		}

		Templates = template.Must(Templates.New(filepath.Base(templateFile)).Parse(string(templateData)))
	}
}


//对数字用大端存储
func encodeBlockNumber(number uint64) []byte {

	enc := make([]byte, 8)
	binary.BigEndian.PutUint64(enc, number)

	return enc
}

//计算区块头hash的key
func headerHashKey(number uint64) []byte {
	return append(append(headerPrefix, encodeBlockNumber(number)...), headerHashSuffix...)
}

//计算区块头RLP的key
func headerRLPKey(headerHash common.Hash, number uint64) []byte {
	return append(append(headerPrefix, encodeBlockNumber(number)...), headerHash.Bytes()...)
}

//计算区块体RLP的key
func bodyRLPKey(headerHash common.Hash, number uint64) []byte {
	return append(append(bodyPrefix, encodeBlockNumber(number)...), headerHash.Bytes()...)
}

//读取区块头的RLP编码
func readBlockHeaderRLP(db *leveldb.DB, headerHash common.Hash, number uint64) rlp.RawValue {

	headerRLP, _ := db.Get(headerRLPKey(headerHash, number), nil)

	//fmt.Println("headerRLP:", headerRLP)

	return headerRLP

}

//读取区块体的RLP编码
func readBlockBodyRLP(db *leveldb.DB, headerHash common.Hash, number uint64) rlp.RawValue {

	bodyRLP, _ := db.Get(bodyRLPKey(headerHash, number), nil)

	//fmt.Println("bodyRLP:", bodyRLP)

	return bodyRLP
}


//从数据库中读区块头
func readBlockHeader(db *leveldb.DB, headerHash common.Hash, number uint64) *types.Header {
	//读取区块头RLP编码
	headerRLPCode := readBlockHeaderRLP(db, headerHash, number)
	if len(headerRLPCode) == 0 {
		return nil
	}

	//将RLP解码为header
	blockHeader := new(types.Header)
	if err := rlp.Decode(bytes.NewReader(headerRLPCode), blockHeader); err != nil {
		log.Panic("Invalid block header RLP!", "err", err)
		return nil
	}

	//fmt.Println("blockHeader:", blockHeader)

	return blockHeader

}

//从数据中读区块体
func readBlockBody(db *leveldb.DB, headerHash common.Hash, number uint64) *types.Body {
	//读取区块体RLP编码
	bodyRLPCode := readBlockBodyRLP(db, headerHash, number)
	if len(bodyRLPCode) == 0 {
		return nil
	}

	//将RLP解码为body
	blockBody := new(types.Body)
	if err := rlp.Decode(bytes.NewReader(bodyRLPCode), blockBody); err != nil {
		log.Panic("Invalid block header RLP!", "err", err)
		return nil
	}

	//fmt.Println("blockBody:", blockBody)

	return blockBody

}

//从数据库读区块
func readBlock(db *leveldb.DB, headerHash common.Hash, number uint64) *types.Block {

	//读区块头
	header := readBlockHeader(db, headerHash, number)
	if header == nil {
		return nil
	}

	//读区块体
	body := readBlockBody(db, headerHash, number)
	if body == nil {
		return nil
	}

	//根据获取的数据合成区块
	return types.NewBlockWithHeader(header).WithBody(body.Transactions, body.Uncles)


}


//根据headerHash和number获取区块
func getBlock(db *leveldb.DB, headerHash common.Hash, number uint64) *types.Block {

	block := readBlock(db, headerHash, number)

	return block

}


func main() {
	//命令行操作
	flag.StringVar(&Options.Host, "host", "", "Hostname to bind web server to")
	flag.IntVar(&Options.Port, "port", 8080, "Port to bind web server to")
	flag.StringVar(&Options.WWWRoot, "www", "www", "Directory to serve")
	flag.StringVar(&Options.TemplatesGlob, "templates", "templates/*", "Templates glob")
	flag.StringVar(&Options.EthEndpoint, "ethendpoint", "http://localhost:8545", "Ethereum node endpoint")
	flag.Parse()

	//初始化模板
	InitTemplates()

	// 启动一个web服务器
	http.Handle("/", HandleTemplates(http.FileServer(http.Dir(Options.WWWRoot))))


	bind := fmt.Sprintf("%v:%d", Options.Host, Options.Port)
	log.Printf("Web server started on %v", bind)

	log.Fatal(http.ListenAndServe(bind, nil))
}
