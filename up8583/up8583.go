/**
银联8583报文,
包含签到，云闪付二维码交易，银联卡小额免密交易
Author:yangyongzhen
QQ:534117529
*/
package up8583

import (
	"errors"
	"fmt"
	"go8583/easy8583"
	"go8583/utils"
	"strconv"
	"strings"
)

type Up8583 struct {
	Ea         *easy8583.Easy8583
	ManNum     string //商户号
	PosNum     string //终端号
	MainKey    string //主密钥
	TPDU       string
	CommSn     int    //通信流水
	RecSn      int    //售卡方系统跟踪号
	PiciNum    []byte //批次号
	LicenceNum []byte

	MacKey string //MAC key

	UpBinNum  string //银行卡卡号
	CardSnNum string //持卡序号
	CardDate  string //卡有效期
	Fd35Data  string //二磁道数据
}

func memcpy(dst, src []byte, size int) {
	for i := 0; i < size; i++ {
		dst[i] = src[i]
	}
	return
}

func equals(src1 []byte, src2 []byte) bool {

	if src1 == nil || src2 == nil {
		return false
	}
	le1 := len(src1)
	le2 := len(src2)
	if le1 != le2 {
		return false
	}
	for i := 0; i < le1; i++ {
		if src1[i] != src2[i] {
			return false
		}
	}
	return true
}

/*
银联8583签到组包
*/
func (up *Up8583) Frame8583QD() {

	s := up.Ea
	field := up.Ea.Field_S

	s.Init8583Fields(field)

	//消息类型
	s.Msgtype[0] = 0x08
	s.Msgtype[1] = 0x00

	//11域，受卡方系统跟踪号BCD 通讯流水
	field[10].Ihave = true
	field[10].Len = 3
	sn := fmt.Sprintf("%06d", up.CommSn)

	field[10].Data = utils.HexStringToBytes(sn)

	//41域，终端号
	field[40].Ihave = true
	field[40].Len = 8
	field[40].Data = []byte(up.PosNum)
	//42域，商户号
	field[41].Ihave = true
	field[41].Len = 15
	field[41].Data = []byte(up.ManNum)
	//60域
	field[59].Ihave = true
	field[59].Len = 0x11
	field[59].Data = make([]byte, 6)
	field[59].Data[0] = 0x00
	memcpy(field[59].Data[1:], up.PiciNum, 3)
	field[59].Data[4] = 0x00
	field[59].Data[5] = 0x30
	//62域
	field[61].Ihave = true
	field[61].Len = 0x25
	field[61].Data = make([]byte, 25)
	str := "Sequence No12"
	memcpy(field[61].Data, []byte(str), 13)
	memcpy(field[61].Data[13:], up.LicenceNum, 4)
	memcpy(field[61].Data[17:], []byte(up.PosNum), 8)

	//63域
	field[62].Ihave = true
	field[62].Len = 0x03
	field[62].Data = make([]byte, 3)
	field[62].Data[0] = 0x30
	field[62].Data[1] = 0x30
	field[62].Data[2] = 0x31
	/*报文组帧，自动组织这些域到Pack的TxBuffer中*/
	s.Pack8583Fields()

	up.CommSn++ //通讯流水每次加一

	//s.PrintFields(up.Ea.Field_S)

}

func (up *Up8583) Ans8583QD(rxbuf []byte, rxlen int) error {

	r := up.Ea
	fields := up.Ea.Field_S
	fieldr := up.Ea.Field_R

	ret := r.Ans8583Fields(rxbuf, rxlen)
	if ret == 0 {
		fmt.Println("解析成功")
		r.PrintFields(fieldr)
	} else {
		fmt.Println("解析失败")
		return errors.New("error,failed to ans..")
	}
	//消息类型判断
	if (r.Msgtype[0] != 0x08) || (r.Msgtype[1] != 0x10) {
		//Log.d(TAG,"消息类型错！");
		return errors.New("error,wrong Msgtype ")
	}
	//应答码判断
	if (fieldr[38].Data[0] != 0x30) || (fieldr[38].Data[1] != 0x30) {
		//Log.d(TAG,"应答码不正确！");
		return errors.New("error,wrong resp code:" + fmt.Sprintf("%02x%02x", fieldr[38].Data[0], fieldr[38].Data[1]))
	}
	//跟踪号比较
	//memcmp
	if !equals(fields[10].Data, fieldr[10].Data) {
		return errors.New("error,wrong comm no ")
	}

	//终端号比较
	if !equals(fields[40].Data, fieldr[40].Data) {
		return errors.New("error,posnum not equal ")
	}
	//商户号比较
	if !equals(fields[41].Data, fieldr[41].Data) {
		return errors.New("error,mannum not equal ")
	}
	//3DES解密PIN KEY
	data := make([]byte, 16)
	memcpy(data, fieldr[61].Data, 16)
	pinkey, err := utils.Des3Decrypt(data, utils.HexStringToBytes(up.MainKey))
	if err != nil {
		return errors.New("1" + err.Error())
	}
	//解密后的结果对8Byte全0做3DES加密运算
	tmp := make([]byte, 8)
	out, err := utils.Des3Encrypt(tmp, pinkey)
	if err != nil {
		return errors.New("2" + err.Error())
	}
	check := make([]byte, 4)
	pincheck := make([]byte, 4)
	memcpy(check, out, 4)
	memcpy(pincheck, fieldr[61].Data[16:], 4)
	if !equals(check, pincheck) {
		return errors.New("error,Er PIK")
	}
	//3DES解密MAC KEY
	memcpy(data, fieldr[61].Data[20:], 16)
	mackey, err := utils.Des3Decrypt(data, utils.HexStringToBytes(up.MainKey))
	if err != nil {
		return errors.New("3" + err.Error())
	}
	out, err = utils.DesEncrypt(tmp, mackey[0:8])
	if err != nil {
		return errors.New("4" + err.Error())
	}
	maccheck := make([]byte, 4)
	memcpy(check, out, 4)
	memcpy(maccheck, fieldr[61].Data[36:], 4)
	if !equals(check, maccheck) {
		return errors.New("error,Er MAC")
	}
	memcpy(up.PiciNum, fieldr[59].Data[1:], 3)
	up.MacKey = utils.BytesToHexString(mackey[0:8])
	fmt.Printf("mackey:%s\n", up.MacKey)
	up.Ea.SetMacKey(up.MacKey)
	return nil
}

func (up *Up8583) Ans8583Qrcode(rxbuf []byte, rxlen int) error {
	r := up.Ea
	fields := up.Ea.Field_S
	fieldr := up.Ea.Field_R

	ret := r.Ans8583Fields(rxbuf, rxlen)
	if ret == 0 {
		fmt.Println("解析成功")
		r.PrintFields(fieldr)
	} else {
		fmt.Println("解析失败")
		return errors.New("error,failed to ans..")
	}
	//消息类型判断
	if (r.Msgtype[0] != 0x02) || (r.Msgtype[1] != 0x10) {
		//Log.d(TAG,"消息类型错！");
		return errors.New("error,wrong Msgtype ")
	}
	//应答码判断
	if (fieldr[38].Data[0] != 0x30) || (fieldr[38].Data[1] != 0x30) {
		//Log.d(TAG,"应答码不正确！");
		return errors.New("error,wrong resp code:" + fmt.Sprintf("%02x%02x", fieldr[38].Data[0], fieldr[38].Data[1]))
	}
	//跟踪号比较
	//memcmp
	if !equals(fields[10].Data, fieldr[10].Data) {
		return errors.New("error,wrong comm no ")
	}

	//终端号比较
	if !equals(fields[40].Data, fieldr[40].Data) {
		return errors.New("error,posnum not equal ")
	}
	//商户号比较
	if !equals(fields[41].Data, fieldr[41].Data) {
		return errors.New("error,mannum not equal ")
	}
	return nil
}

/*
银联8583 二维码交易组包
qrcode:二维码内容
money:交易金额
recSn:交易流水
*/
func (up *Up8583) Frame8583Qrcode(qrcode string, money int, recSn int) {

	s := up.Ea
	field := up.Ea.Field_S

	s.Init8583Fields(field)

	//消息类型
	s.Msgtype[0] = 0x02
	s.Msgtype[1] = 0x00

	//3域 交易处理码
	field[2].Ihave = true
	field[2].Len = 3
	field[2].Data = make([]byte, 3)
	//4域 交易金额
	field[3].Ihave = true
	field[3].Len = 6
	field[3].Data = utils.HexStringToBytes(fmt.Sprintf("%012d", money))
	//11域，受卡方系统跟踪号BCD 通讯流水
	field[10].Ihave = true
	field[10].Len = 3
	sn := fmt.Sprintf("%06d", recSn)

	field[10].Data = utils.HexStringToBytes(sn)

	//22域
	field[21].Ihave = true
	field[21].Len = 2
	field[21].Data = []byte{0x03, 0x20}
	//25域
	field[24].Ihave = true
	field[24].Len = 1
	field[24].Data = make([]byte, 1)

	//41域，终端号
	field[40].Ihave = true
	field[40].Len = 8
	field[40].Data = []byte(up.PosNum)
	//42域，商户号
	field[41].Ihave = true
	field[41].Len = 15
	field[41].Data = []byte(up.ManNum)

	//49域 交易货币代码
	field[48].Ihave = true
	field[48].Len = 3
	field[48].Data = []byte{0x31, 0x35, 0x36}
	//59域，扫码的数据
	field[58].Ihave = true
	field[58].Len = 0x24
	field[58].Data = make([]byte, 24)
	field[58].Data[0] = 'A' //TAG+Len(019)
	field[58].Data[1] = '3'
	field[58].Data[2] = '0'
	field[58].Data[3] = '1'
	field[58].Data[4] = '9'
	memcpy(field[58].Data[5:], []byte(qrcode), 19)

	//60域
	field[59].Ihave = true
	field[59].Len = 0x13
	field[59].Data = make([]byte, 7)
	field[59].Data[0] = 0x22
	memcpy(field[59].Data[1:], up.PiciNum, 3)
	field[59].Data[4] = 0x00
	field[59].Data[5] = 0x06
	field[59].Data[6] = 0x00

	//MAC，64域
	field[63].Ihave = true
	field[63].Len = 0x08
	field[63].Data = make([]byte, 8)
	//这个域要求填MAC，只需按这样填，MAC的计算在pack8583Fields自动完成了
	/*报文组帧，自动组织这些域到Pack的TxBuffer中*/
	s.Pack8583Fields()

	//CommSn++ //通讯流水每次加一

	//s.PrintFields(up.Ea.Field_S)

}

func NewUp8583() *Up8583 {

	var up = new(Up8583)
	up.Ea = easy8583.New8583()
	up.TPDU = "6000000001"
	up.ManNum = "000000000000000"
	up.PosNum = "00000000"
	up.MainKey = "00000000000000000000000000000000"
	up.CommSn = 1
	up.RecSn = 1 //终端交易流水，连续，且不能重复
	up.PiciNum = make([]byte, 3)
	up.LicenceNum = []byte{0x33, 0x30, 0x36, 0x30}
	up.MacKey = "0000000000000000"
	up.Ea.Tpdu = utils.HexStringToBytes(up.TPDU)
	return up

}

/**
Setup
初始化参数配置
manNum:商户号
posNum:终端号
mainKey:主密钥
*/
func (up *Up8583) Setup(manNum, posNum, mainKey, tpdu string) {
	up.TPDU = tpdu
	up.ManNum = manNum
	up.PosNum = posNum
	up.MainKey = mainKey
	up.Ea.Tpdu = utils.HexStringToBytes(up.TPDU)
}

/*
银联8583 电子现金交易组包
*/
//获取55域 IC卡数据域
/*
9F26 08
9F27 01
9F10
9F37 04
9F36 02
95 05
9A 03
9C 01
9F02 06
5F2A 02
82 02
9F1A 02
9F03 06
9F33 03
9F1E 08
84
9F09 02
9F41 04
9F34 03
9F35 01
9F63 10
9F74 06
8A 02
*/

func (up *Up8583) Frame8583UpCash(cardbin string, money int, cardvailddata string, cardholdsn string, field55 []byte) {
	s := up.Ea
	field := up.Ea.Field_S

	s.Init8583Fields(field)

	//消息类型
	s.Msgtype[0] = 0x02
	s.Msgtype[1] = 0x00
	//2域 卡号
	field[1].Ihave = true
	tmp := fmt.Sprintf("%02d", len(cardbin))
	t, _ := strconv.ParseInt(tmp, 16, 16)
	if len(cardbin)%2 != 0 {
		cardbin += "0"
	}
	field[1].Len = int(t)
	field[1].Data = utils.HexStringToBytes(cardbin)
	//3域 交易处理码
	field[2].Ihave = true
	field[2].Len = 3
	field[2].Data = make([]byte, 3)
	//4域 交易金额
	field[3].Ihave = true
	field[3].Len = 6
	field[3].Data = utils.HexStringToBytes(fmt.Sprintf("%012d", money))
	//11域，受卡方系统跟踪号BCD 通讯流水
	field[10].Ihave = true
	field[10].Len = 3
	sn := fmt.Sprintf("%06d", up.RecSn)

	field[10].Data = utils.HexStringToBytes(sn)

	//14域 卡有效期，能获取到时存在
	if len(cardvailddata) > 0 {
		field[13].Ihave = true
		field[13].Len = 2
		field[13].Data = utils.HexStringToBytes(cardvailddata)
	}

	//22域
	field[21].Ihave = true
	field[21].Len = 2
	field[21].Data = []byte{0x07, 0x20}
	//23域，卡序列号 能获取时存在
	if len(cardholdsn) > 0 {
		field[22].Ihave = true
		field[22].Len = 2
		field[22].Data = utils.HexStringToBytes(cardholdsn)
	}
	//25域
	field[24].Ihave = true
	field[24].Len = 1
	field[24].Data = make([]byte, 1)

	//41域，终端号
	field[40].Ihave = true
	field[40].Len = 8
	field[40].Data = []byte(up.PosNum)
	//42域，商户号
	field[41].Ihave = true
	field[41].Len = 15
	field[41].Data = []byte(up.ManNum)

	//49域 交易货币代码
	field[48].Ihave = true
	field[48].Len = 3
	field[48].Data = []byte{0x31, 0x35, 0x36}

	//55域 IC卡数据域
	field[54].Ihave = true
	tmp = fmt.Sprintf("%04d", len(field55))
	b := utils.HexStringToBytes(tmp)
	field[54].Len = int(b[0])<<8 | int(b[1])
	field[54].Data = field55
	//60域
	field[59].Ihave = true
	field[59].Len = 0x13
	field[59].Data = make([]byte, 7)
	field[59].Data[0] = 0x36
	memcpy(field[59].Data[1:], up.PiciNum, 3)
	field[59].Data[4] = 0x00
	field[59].Data[5] = 0x06
	field[59].Data[6] = 0x00
	//63域
	field[62].Ihave = true
	field[62].Len = 0x03
	field[62].Data = make([]byte, 3)
	memcpy(field[62].Data, []byte("CUP"), 3)
	//MAC，64域
	field[63].Ihave = true
	field[63].Len = 0x08
	field[63].Data = make([]byte, 8)
	//这个域要求填MAC，只需按这样填，MAC的计算在pack8583Fields自动完成了
	/*报文组帧，自动组织这些域到Pack的TxBuffer中*/
	s.Pack8583Fields()
}

func (up *Up8583) Ans8583UpCash(rxbuf []byte, rxlen int) error {

	r := up.Ea
	fields := up.Ea.Field_S
	fieldr := up.Ea.Field_R

	ret := r.Ans8583Fields(rxbuf, rxlen)
	if ret == 0 {
		fmt.Println("解析成功")
		r.PrintFields(fieldr)
	} else {
		fmt.Println("解析失败")
		r.PrintFields(fieldr)
		return errors.New("error,failed to ans..")
	}
	//消息类型判断
	if (r.Msgtype[0] != 0x02) || (r.Msgtype[1] != 0x10) {
		//Log.d(TAG,"消息类型错！");
		return errors.New("error,wrong Msgtype ")
	}
	//应答码判断
	if (fieldr[38].Data[0] != 0x30) || (fieldr[38].Data[1] != 0x30) {
		//Log.d(TAG,"应答码不正确！");
		return errors.New("error,wrong resp code:" + fmt.Sprintf("%02x%02x", fieldr[38].Data[0], fieldr[38].Data[1]))
	}
	//跟踪号比较
	//memcmp
	if !equals(fields[10].Data, fieldr[10].Data) {
		return errors.New("error,wrong comm no ")
	}

	//终端号比较
	if !equals(fields[40].Data, fieldr[40].Data) {
		return errors.New("error,posnum not equal ")
	}
	//商户号比较
	if !equals(fields[41].Data, fieldr[41].Data) {
		return errors.New("error,mannum not equal ")
	}
	//MAC验证
	mac, err := easy8583.UpGetMac(rxbuf[13:], rxlen-13-8, up.Ea.MacKey)
	if err != nil {
		fmt.Println(err)
		panic("calc mac error!")
	}
	if !equals(fieldr[63].Data, mac) {
		return errors.New("error,mac check err")
	}

	return nil
}

/*
*银联双免组包
*输入参数：交易金额，日期时间 卡上GPO返回的数据
 */
func getfield55(in []byte, inlen int) []byte {
	return nil

}

func (up *Up8583) Frame8583Quics(money int, dealtime string, field55 []byte) {
	s := up.Ea
	field := up.Ea.Field_S

	s.Init8583Fields(field)

	//消息类型
	s.Msgtype[0] = 0x02
	s.Msgtype[1] = 0x00
	//2域 卡号
	field[1].Ihave = true
	tmp := fmt.Sprintf("%02d", len(up.UpBinNum))
	t, _ := strconv.ParseInt(tmp, 16, 16)
	if len(up.UpBinNum)%2 != 0 {
		up.UpBinNum += "0"
	}
	field[1].Len = int(t)
	field[1].Data = utils.HexStringToBytes(up.UpBinNum)
	//3域 交易处理码
	field[2].Ihave = true
	field[2].Len = 3
	field[2].Data = make([]byte, 3)
	//4域 交易金额
	field[3].Ihave = true
	field[3].Len = 6
	field[3].Data = utils.HexStringToBytes(fmt.Sprintf("%012d", money))
	//11域，受卡方系统跟踪号BCD 通讯流水
	field[10].Ihave = true
	field[10].Len = 3
	sn := fmt.Sprintf("%06d", up.RecSn)

	field[10].Data = utils.HexStringToBytes(sn)

	//14域 卡有效期，能获取到时存在
	if len(up.CardDate) > 0 {
		field[13].Ihave = true
		field[13].Len = 2
		field[13].Data = utils.HexStringToBytes(up.CardDate)
	}

	//22域
	field[21].Ihave = true
	field[21].Len = 2
	field[21].Data = []byte{0x07, 0x20}
	//23域，卡序列号 能获取时存在
	if len(up.CardSnNum) > 0 {

		if strings.EqualFold(up.CardSnNum, "01") {
			field[22].Ihave = true
			field[22].Len = 2
			field[22].Data = make([]byte, 2)
			field[22].Data[1] = 0x01
		} else {
			field[22].Ihave = true
			field[22].Len = 2
			field[22].Data = make([]byte, 2)
			field[22].Data[1] = 0x00
		}

	}
	//25域
	field[24].Ihave = true
	field[24].Len = 1
	field[24].Data = make([]byte, 1)

	//35域 二磁道数据
	tmplen := len(up.Fd35Data)
	if tmplen > 0 {
		field[34].Ihave = true
		if strings.EqualFold(up.Fd35Data[tmplen-1:], "f") {
			st := fmt.Sprintf("%02d", tmplen)
			out, _ := strconv.ParseInt(st, 16, 16)
			tmplen = int(out)
			field[34].Len = tmplen - 1
		} else {
			st := fmt.Sprintf("%02d", tmplen)
			out, _ := strconv.ParseInt(st, 16, 16)
			tmplen = int(out)
			field[34].Len = tmplen
		}
		field[34].Data = utils.HexStringToBytes(up.Fd35Data)
	}
	//41域，终端号
	field[40].Ihave = true
	field[40].Len = 8
	field[40].Data = []byte(up.PosNum)
	//42域，商户号
	field[41].Ihave = true
	field[41].Len = 15
	field[41].Data = []byte(up.ManNum)

	//49域 交易货币代码
	field[48].Ihave = true
	field[48].Len = 3
	field[48].Data = []byte{0x31, 0x35, 0x36}

	//55域 IC卡数据域
	field[54].Ihave = true
	tmp = fmt.Sprintf("%04d", len(field55))
	b := utils.HexStringToBytes(tmp)
	field[54].Len = int(b[0])<<8 | int(b[1])
	field[54].Data = field55
	//60域
	field[59].Ihave = true
	field[59].Len = 0x13
	field[59].Data = make([]byte, 7)
	field[59].Data[0] = 0x22
	memcpy(field[59].Data[1:], up.PiciNum, 3)
	field[59].Data[4] = 0x00
	field[59].Data[5] = 0x06
	field[59].Data[6] = 0x00

	//MAC，64域
	field[63].Ihave = true
	field[63].Len = 0x08
	field[63].Data = make([]byte, 8)
	//这个域要求填MAC，只需按这样填，MAC的计算在pack8583Fields自动完成了
	/*报文组帧，自动组织这些域到Pack的TxBuffer中*/
	//s.PrintFields(field)

	s.Pack8583Fields()
}

func (up *Up8583) Ans8583Quics(rxbuf []byte, rxlen int) error {

	r := up.Ea
	fields := up.Ea.Field_S
	fieldr := up.Ea.Field_R

	ret := r.Ans8583Fields(rxbuf, rxlen)
	if ret == 0 {
		fmt.Println("解析成功")
		r.PrintFields(fieldr)
	} else {
		fmt.Println("解析失败")
		r.PrintFields(fieldr)
		return errors.New("error,failed to ans..")
	}
	//消息类型判断
	if (r.Msgtype[0] != 0x02) || (r.Msgtype[1] != 0x10) {
		//Log.d(TAG,"消息类型错！");
		return errors.New("error,wrong Msgtype ")
	}
	//应答码判断
	if (fieldr[38].Data[0] != 0x30) || (fieldr[38].Data[1] != 0x30) {
		//Log.d(TAG,"应答码不正确！");
		return errors.New("error,wrong resp code:" + fmt.Sprintf("%02x%02x", fieldr[38].Data[0], fieldr[38].Data[1]))
	}
	//跟踪号比较
	//memcmp
	if !equals(fields[10].Data, fieldr[10].Data) {
		return errors.New("error,wrong comm no ")
	}

	//终端号比较
	if !equals(fields[40].Data, fieldr[40].Data) {
		return errors.New("error,posnum not equal ")
	}
	//商户号比较
	if !equals(fields[41].Data, fieldr[41].Data) {
		return errors.New("error,mannum not equal ")
	}
	//MAC验证
	mac, err := easy8583.UpGetMac(rxbuf[13:], rxlen-13-8, up.Ea.MacKey)
	if err != nil {
		fmt.Println(err)
		panic("calc mac error!")
	}
	if !equals(fieldr[63].Data, mac) {
		return errors.New("error,mac check err")
	}

	return nil
}

/**
使用demo
*/
func main() {

	fmt.Println("test...")

	up := NewUp8583()
	up.Setup("888888888888888", "12345678", "11111111111111111111111111111111", "6005010000")
	//up.Frame8583QD()

	//recvstr := "007960000001386131003111080810003800010AC0001450021122130107200800085500323231333031343931333239303039393939393930363030313433303137303131393939390011000005190030004046F161A743497B32EAC760DF5EA57DF5900ECCE3977731A7EA402DDF0000000000000000CFF1592A"

	//recv := utils.HexStringToBytes(recvstr)
	//ret := up.Ea.Ans8583Fields(recv, len(recv))
	//if ret == 0 {
	// 	fmt.Println("解析成功")
	// 	up.Ea.PrintFields(up.Ea.Field_R)
	// } else {
	// 	fmt.Println("解析失败")
	// }

	up.Frame8583QD()
	up.Ea.PrintFields(up.Ea.Field_S)
	//
	//fmt.Println(utils.BytesToHexString(up.Ea.Txbuf))
	//rxbuf, err := utils.UpHttpsPost(Url, up.Ea.Txbuf)
	// err = up.Ans8583QD(rxbuf, rxlen)
	// if err == nil {
	// 	log.Println("签到成功")
	// }tring(up.Ea.Txbuf))
	up.Frame8583Qrcode("6220485073630469936", 1, 1)
	up.Ea.PrintFields(up.Ea.Field_S)

}
