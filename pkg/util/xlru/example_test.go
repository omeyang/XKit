package xlru_test

import (
	"fmt"
	"time"

	"github.com/omeyang/xkit/pkg/util/xlru"
)

func Example() {
	// 创建一个最多存储 1000 条目、TTL 为 5 分钟的缓存
	cache, err := xlru.New[string, int](xlru.Config{
		Size: 1000,
		TTL:  5 * time.Minute,
	})
	if err != nil {
		panic(err)
	}

	// 设置值
	cache.Set("user:123", 42)

	// 获取值
	if val, ok := cache.Get("user:123"); ok {
		fmt.Println("Found:", val)
	}

	// 检查是否存在
	if cache.Contains("user:123") {
		fmt.Println("Key exists")
	}

	// 删除
	cache.Delete("user:123")

	// 检查长度
	fmt.Println("Length:", cache.Len())

	// Output:
	// Found: 42
	// Key exists
	// Length: 0
}

func Example_withEvictionCallback() {
	// 创建带淘汰回调的缓存
	cache, err := xlru.New(xlru.Config{Size: 2, TTL: time.Minute},
		xlru.WithOnEvicted(func(key string, value int) {
			fmt.Printf("Evicted: %s=%d\n", key, value)
		}))
	if err != nil {
		panic(err)
	}

	// 填满缓存
	cache.Set("key1", 100)
	cache.Set("key2", 200)

	// 添加新条目，触发淘汰
	cache.Set("key3", 300)

	fmt.Println("Length:", cache.Len())

	// Output:
	// Evicted: key1=100
	// Length: 2
}

func Example_pointerValues() {
	type UserData struct {
		Name string
		Age  int
	}

	// 使用指针类型作为值
	cache, err := xlru.New[string, *UserData](xlru.Config{
		Size: 100,
		TTL:  time.Minute,
	})
	if err != nil {
		panic(err)
	}

	// 存储指针
	cache.Set("user:1", &UserData{Name: "Alice", Age: 30})

	// 获取并使用
	if user, ok := cache.Get("user:1"); ok {
		fmt.Printf("User: %s, Age: %d\n", user.Name, user.Age)
	}

	// Output:
	// User: Alice, Age: 30
}

func Example_keys() {
	cache, err := xlru.New[string, int](xlru.Config{
		Size: 10,
		TTL:  time.Minute,
	})
	if err != nil {
		panic(err)
	}

	cache.Set("a", 1)
	cache.Set("b", 2)
	cache.Set("c", 3)

	keys := cache.Keys()
	fmt.Println("Number of keys:", len(keys))

	// Output:
	// Number of keys: 3
}
