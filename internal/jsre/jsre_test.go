
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:40</date>
//</624342642624303104>


package jsre

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/robertkrimen/otto"
)

type testNativeObjectBinding struct{}

type msg struct {
	Msg string
}

func (no *testNativeObjectBinding) TestMethod(call otto.FunctionCall) otto.Value {
	m, err := call.Argument(0).ToString()
	if err != nil {
		return otto.UndefinedValue()
	}
	v, _ := call.Otto.ToValue(&msg{m})
	return v
}

func newWithTestJS(t *testing.T, testjs string) (*JSRE, string) {
	dir, err := ioutil.TempDir("", "jsre-test")
	if err != nil {
		t.Fatal("cannot create temporary directory:", err)
	}
	if testjs != "" {
		if err := ioutil.WriteFile(path.Join(dir, "test.js"), []byte(testjs), os.ModePerm); err != nil {
			t.Fatal("cannot create test.js:", err)
		}
	}
	return New(dir, os.Stdout), dir
}

func TestExec(t *testing.T) {
	jsre, dir := newWithTestJS(t, `msg = "testMsg"`)
	defer os.RemoveAll(dir)

	err := jsre.Exec("test.js")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	val, err := jsre.Run("msg")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !val.IsString() {
		t.Errorf("expected string value, got %v", val)
	}
	exp := "testMsg"
	got, _ := val.ToString()
	if exp != got {
		t.Errorf("expected '%v', got '%v'", exp, got)
	}
	jsre.Stop(false)
}

func TestNatto(t *testing.T) {
	jsre, dir := newWithTestJS(t, `setTimeout(function(){msg = "testMsg"}, 1);`)
	defer os.RemoveAll(dir)

	err := jsre.Exec("test.js")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	val, err := jsre.Run("msg")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !val.IsString() {
		t.Errorf("expected string value, got %v", val)
	}
	exp := "testMsg"
	got, _ := val.ToString()
	if exp != got {
		t.Errorf("expected '%v', got '%v'", exp, got)
	}
	jsre.Stop(false)
}

func TestBind(t *testing.T) {
	jsre := New("", os.Stdout)
	defer jsre.Stop(false)

	jsre.Bind("no", &testNativeObjectBinding{})

	_, err := jsre.Run(`no.TestMethod("testMsg")`)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestLoadScript(t *testing.T) {
	jsre, dir := newWithTestJS(t, `msg = "testMsg"`)
	defer os.RemoveAll(dir)

	_, err := jsre.Run(`loadScript("test.js")`)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	val, err := jsre.Run("msg")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !val.IsString() {
		t.Errorf("expected string value, got %v", val)
	}
	exp := "testMsg"
	got, _ := val.ToString()
	if exp != got {
		t.Errorf("expected '%v', got '%v'", exp, got)
	}
	jsre.Stop(false)
}

