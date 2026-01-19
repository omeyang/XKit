# 命名准则

## 核心原则

1. **见名知义** - 不看注释就知道用途
2. **简短准确** - 避免冗长，但必须准确
3. **禁止泛化** - 不用 manager/service/handler/util 等模糊词
4. **避免包名重复** - 导出名不要包含包名
5. **符合 Uber Go 规范**

---

## 重要：避免包名重复

导出的类型、函数、接口名**不要包含包名**，因为调用时会重复。

### 为什么？

```go
// ❌ 错误：包名重复
package xasset

type AssetRepository interface { ... }
func QueryAsset(ip string) { ... }

// 调用时：
xasset.AssetRepository  // 重复了 Asset
xasset.QueryAsset()     // 重复了 Asset
```

```go
// ✅ 正确：简洁清晰
package xasset

type Repository interface { ... }
func Query(ip string) { ... }

// 调用时：
xasset.Repository  // 清晰
xasset.Query()     // 简洁
```

**GoLand/GoCode 会提示告警**：`exported type/func should not have package name prefix`

---

## 目录（模块路径片段）

### 规则
- kebab-case（短横线分隔）
- 见名知义
- 避免缩写

### ✅ 正确
```
string-utils
auth-server
kv-store
```

### ❌ 错误
```
stringUtils   // 不用驼峰
string_utils  // 不用下划线
utils         // 太泛化
```

---

## 包（Package）

### 规则
- 统一使用 `x` 前缀
- 小写字母
- 尽量单词、语义清晰
- 无下划线、无短横线、无驼峰
- 禁止使用泛化词（manager/service/handler/util/helper/common/base/data）

### ✅ 正确
```
xstr
xauth
xkv
xcrypto
xmetrics
```

### ❌ 错误
```
metrics       // 缺少 x 前缀
x_string      // 不用下划线
x-str         // 不用短横线
xassetManager // 不用驼峰
xutil         // 太泛化
```

---

## 模块路径（go.mod）

### 规则
- 允许 kebab-case
- 与目录命名一致

### ✅ 正确
```
module github.com/yourname/demo-proj
```

### ❌ 错误
```
module github.com/yourname/demo_proj // 不用下划线
```

---

## 文件

### 规则
- snake_case + `.go`
- 见名知义
- 避免缩写

### ✅ 正确
```
asset_matcher.go
query_by_ip.go
branch_tree.go
```

### ❌ 错误
```
assetMatcher.go  // 不用驼峰
asset-matcher.go // 不用短横线
am.go            // 不用缩写
utils.go         // 太泛化
```

---

## 类型（Type）

### 规则
- PascalCase
- 名词或名词短语
- 准确描述用途
- 禁止 Manager/Service/Handler 后缀
- **不要包含包名**

### ✅ 正确
```go
// 在 xasset 包中
type Matcher struct { ... }      // 调用：xasset.Matcher
type Repository struct { ... }   // 调用：xasset.Repository
type Locator struct { ... }      // 调用：xasset.Locator
```

### ❌ 错误
```go
// 在 xasset 包中
type AssetMatcher struct { ... }     // 调用：xasset.AssetMatcher（包名重复）
type AssetRepository struct { ... }  // 调用：xasset.AssetRepository（包名重复）
type AssetManager struct { ... }     // 太泛化 + 包名重复
type AssetService struct { ... }     // 太泛化 + 包名重复
type Handler struct { ... }          // 完全不知道干什么
type Data struct { ... }             // 太模糊
```

---

## 接口（Interface）

### 规则
- PascalCase
- 单方法接口用 `-er` 后缀
- 多方法接口用名词
- **不要包含包名**
- 不用 I 前缀或 Interface 后缀

### ✅ 正确
```go
// 在 xasset 包中
type Repository interface { ... }  // 调用：xasset.Repository
type Matcher interface { ... }     // 调用：xasset.Matcher
type Locator interface { ... }     // 调用：xasset.Locator
```

### ❌ 错误
```go
// 在 xasset 包中
type AssetRepository interface { ... }  // 包名重复
type IRepository interface { ... }      // 不用 I 前缀
type RepositoryInterface interface { ... } // 不用 Interface 后缀
```

---

## 函数/方法

### 规则
- PascalCase（公开）/ camelCase（私有）
- 动词开头
- 准确描述操作
- **不要包含包名**

### ✅ 正确
```go
// 在 xasset 包中
func Query(ip string) (*Asset, error) { ... }           // 调用：xasset.Query()
func Create(a *Asset) error { ... }                     // 调用：xasset.Create()
func MatchByGroup(groupID string) ([]*Asset, error) { ... } // 调用：xasset.MatchByGroup()
func validateInput(ip string) error { ... }             // 私有函数
```

### ❌ 错误
```go
// 在 xasset 包中
func QueryAsset(ip string) { ... }       // 包名重复：xasset.QueryAsset()
func AssetQuery(ip string) { ... }       // 包名重复：xasset.AssetQuery()
func CreateAsset(a *Asset) { ... }       // 包名重复：xasset.CreateAsset()
func Handle(data interface{}) { ... }    // 处理什么？
func Process(data interface{}) { ... }   // 处理什么？
func Do() { ... }                        // 做什么？
func GetData() { ... }                   // 获取什么数据？
```

---

## 变量

### 规则
- camelCase
- 简短但清晰
- 缩写全大写（ID/IP/HTTP）

### ✅ 正确
```go
assetID
branchTree
userIP
httpClient
```

### ❌ 错误
```go
AssetID       // 非导出变量不用 PascalCase
assetId       // 缩写不全大写
data          // 太模糊
temp          // 太模糊
```

---

## 常量

### 规则
- PascalCase 或全大写（枚举用 PascalCase）
- 有意义的名字

### ✅ 正确
```go
const MaxRetryCount = 3
const DefaultTimeout = 30 * time.Second

const (
    StatusActive   = "active"    // 枚举
    StatusInactive = "inactive"
)
```

### ❌ 错误
```go
const MAX_RETRY = 3    // 不用全大写+下划线
const max = 3          // 太模糊
```

---

## 特殊命名

### 接收者（Receiver）
- 1-2个字母缩写
- 统一使用相同缩写

```go
func (a *Asset) Create() { ... }        // 统一用 a
func (a *Asset) Update() { ... }        // 统一用 a
```

### 上下文（Context）
- 第一个参数
- 命名为 `ctx`

```go
func QueryAsset(ctx context.Context, ip string) (*Asset, error) { ... }
```

### 错误（Error）
- 命名为 `err`
- 自定义错误类型用 `Err` 前缀

```go
var ErrNotFound = errors.New("not found")
var ErrInvalidInput = errors.New("invalid input")
```

---

## 禁止列表

### 绝对禁止的词
- `Manager` - 用具体动作代替（如 `Matcher`、`Locator`）
- `Service` - 用具体职责代替（如 `Repository`、`Validator`）
- `Handler` - 用具体处理代替（如 `AssetQueryHandler` → `AssetQuerier`）
- `Util/Utils` - 用具体功能代替（如 `StringValidator`、`TimeFormatter`）
- `Helper` - 用具体功能代替
- `Common` - 用具体功能代替
- `Base` - 用具体功能代替
- `Data` - 用具体类型代替

### 替代方案
```
AssetManager    →  AssetRepository / AssetMatcher
UserService     →  UserRepository / UserValidator
DataHandler     →  DataProcessor / DataTransformer
StringUtil      →  StringValidator / StringParser
```

---

## 检查清单

- [ ] 包名 `x` 前缀、小写、无符号
- [ ] 文件名 snake_case
- [ ] 类型名 PascalCase、见名知义
- [ ] 函数名动词开头、准确描述
- [ ] 变量名 camelCase、清晰
- [ ] **导出名不包含包名**（避免 xasset.AssetQuery）
- [ ] 没有使用禁止词（Manager/Service/Handler/Util）
- [ ] 缩写全大写（ID/IP/HTTP/URL）
- [ ] 接收者名称统一
