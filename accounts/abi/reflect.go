
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:26</date>
//</624342583904047104>


package abi

import (
	"fmt"
	"reflect"
	"strings"
)

//间接递归地取消对值的引用，直到它得到值为止
//或者找到一个大的.int
func indirect(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Ptr && v.Elem().Type() != derefbigT {
		return indirect(v.Elem())
	}
	return v
}

//ReflectIntKind返回使用给定大小和
//不拘一格
func reflectIntKindAndType(unsigned bool, size int) (reflect.Kind, reflect.Type) {
	switch size {
	case 8:
		if unsigned {
			return reflect.Uint8, uint8T
		}
		return reflect.Int8, int8T
	case 16:
		if unsigned {
			return reflect.Uint16, uint16T
		}
		return reflect.Int16, int16T
	case 32:
		if unsigned {
			return reflect.Uint32, uint32T
		}
		return reflect.Int32, int32T
	case 64:
		if unsigned {
			return reflect.Uint64, uint64T
		}
		return reflect.Int64, int64T
	}
	return reflect.Ptr, bigT
}

//mustArrayToBytesSlice创建与值大小完全相同的新字节片
//并将值中的字节复制到新切片。
func mustArrayToByteSlice(value reflect.Value) reflect.Value {
	slice := reflect.MakeSlice(reflect.TypeOf([]byte{}), value.Len(), value.Len())
	reflect.Copy(slice, value)
	return slice
}

//设置尝试通过设置、复制或其他方式将SRC分配给DST。
//
//当涉及到任务时，set要宽松一点，而不是强制
//严格的规则集为bare`reflect`的规则集。
func set(dst, src reflect.Value, output Argument) error {
	dstType := dst.Type()
	srcType := src.Type()
	switch {
	case dstType.AssignableTo(srcType):
		dst.Set(src)
	case dstType.Kind() == reflect.Interface:
		dst.Set(src)
	case dstType.Kind() == reflect.Ptr:
		return set(dst.Elem(), src, output)
	default:
		return fmt.Errorf("abi: cannot unmarshal %v in to %v", src.Type(), dst.Type())
	}
	return nil
}

//RequiresSignable确保“dest”是指针，而不是接口。
func requireAssignable(dst, src reflect.Value) error {
	if dst.Kind() != reflect.Ptr && dst.Kind() != reflect.Interface {
		return fmt.Errorf("abi: cannot unmarshal %v into %v", src.Type(), dst.Type())
	}
	return nil
}

//RequireUnpackKind验证将“args”解包为“kind”的前提条件
func requireUnpackKind(v reflect.Value, t reflect.Type, k reflect.Kind,
	args Arguments) error {

	switch k {
	case reflect.Struct:
	case reflect.Slice, reflect.Array:
		if minLen := args.LengthNonIndexed(); v.Len() < minLen {
			return fmt.Errorf("abi: insufficient number of elements in the list/array for unpack, want %d, got %d",
				minLen, v.Len())
		}
	default:
		return fmt.Errorf("abi: cannot unmarshal tuple into %v", t)
	}
	return nil
}

//mapabitoStringField将abi映射到结构字段。
//第一轮：对于每个包含“abi:”标记的可导出字段
//这个字段名存在于参数中，将它们配对在一起。
//第二轮：对于每个尚未链接的参数字段，
//如果变量存在且尚未映射，则查找该变量应映射到的对象。
//用过，配对。
func mapAbiToStructFields(args Arguments, value reflect.Value) (map[string]string, error) {

	typ := value.Type()

	abi2struct := make(map[string]string)
	struct2abi := make(map[string]string)

//第一轮~~~
	for i := 0; i < typ.NumField(); i++ {
		structFieldName := typ.Field(i).Name

//跳过私有结构字段。
		if structFieldName[:1] != strings.ToUpper(structFieldName[:1]) {
			continue
		}

//跳过没有abi:“”标记的字段。
		var ok bool
		var tagName string
		if tagName, ok = typ.Field(i).Tag.Lookup("abi"); !ok {
			continue
		}

//检查标签是否为空。
		if tagName == "" {
			return nil, fmt.Errorf("struct: abi tag in '%s' is empty", structFieldName)
		}

//检查哪个参数字段与ABI标记匹配。
		found := false
		for _, abiField := range args.NonIndexed() {
			if abiField.Name == tagName {
				if abi2struct[abiField.Name] != "" {
					return nil, fmt.Errorf("struct: abi tag in '%s' already mapped", structFieldName)
				}
//配对他们
				abi2struct[abiField.Name] = structFieldName
				struct2abi[structFieldName] = abiField.Name
				found = true
			}
		}

//检查此标记是否已映射。
		if !found {
			return nil, fmt.Errorf("struct: abi tag '%s' defined but not found in abi", tagName)
		}

	}

//第二轮~~~
	for _, arg := range args {

		abiFieldName := arg.Name
		structFieldName := capitalise(abiFieldName)

		if structFieldName == "" {
			return nil, fmt.Errorf("abi: purely underscored output cannot unpack to struct")
		}

//这个ABI已经配对了，跳过它…除非存在另一个尚未分配的
//具有相同字段名的结构字段。如果是，则引发错误：
//ABI：[“name”：“value”]
//结构value*big.int，value1*big.int`abi:“value”`
		if abi2struct[abiFieldName] != "" {
			if abi2struct[abiFieldName] != structFieldName &&
				struct2abi[structFieldName] == "" &&
				value.FieldByName(structFieldName).IsValid() {
				return nil, fmt.Errorf("abi: multiple variables maps to the same abi field '%s'", abiFieldName)
			}
			continue
		}

//如果此结构字段已配对，则返回错误。
		if struct2abi[structFieldName] != "" {
			return nil, fmt.Errorf("abi: multiple outputs mapping to the same struct field '%s'", structFieldName)
		}

		if value.FieldByName(structFieldName).IsValid() {
//配对他们
			abi2struct[abiFieldName] = structFieldName
			struct2abi[structFieldName] = abiFieldName
		} else {
//不是成对的，而是按使用进行注释，以检测
//ABI：[“name”：“value”，“name”：“value”]
//结构值*big.int
			struct2abi[structFieldName] = abiFieldName
		}

	}

	return abi2struct, nil
}

