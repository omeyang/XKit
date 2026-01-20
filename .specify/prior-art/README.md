# XKit 标准与先验知识索引

本目录收录 XKit 项目采用的统一标准文档和与实现紧密相关的先验知识，作为单一可信入口，避免分散和重复描述。

---

## 1. 目录结构

```text
prior-art/
├── 01-standards/                   # 项目采用的统一标准
│   ├── 01-documentation/           # 文档金标准
│   ├── 02-code-quality/            # 代码质量标准
│   ├── 03-testing/                 # 测试标准
│   └── 04-go-ecosystem/            # Go 依赖与生态相关标准
└── README.md                   # 本索引
```

---

## 2. 标准文档入口

- **文档金标准**：`.specify/prior-art/01-standards/01-documentation/`  
  定义 `.specify` 目录下技术文档的质量要求（编号、职责、溯源等）。
- **代码质量标准**：`.specify/prior-art/01-standards/02-code-quality/`  
  约定命名规则、设计原则和提交前检查清单。
- **测试标准**：`.specify/prior-art/01-standards/03-testing/`  
  定义 TDD 流程、覆盖率要求和测试结构。
- **Go 依赖标准**：`.specify/prior-art/01-standards/04-go-ecosystem/01-go-dependencies.md`  
  约定 Go module 路径、依赖引入与升级策略。

---

## 3. 适用范围

- `.specify/specs/*/analysis/`：技术分析文档引用本目录中的相关标准作为约束依据。
- `.specify/specs/*/design/`：设计文档中的约束与决策需与本目录标准保持一致。
- `.specify/prior-art/`：新增先验知识文档时必须：
  - 明确单一职责；
  - 标注与标准文档的依赖关系；
  - 避免描述历史版本实现，历史信息放入对应 Feature 的 ADR / archived 目录。
