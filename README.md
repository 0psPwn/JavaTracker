# JavaTracker

JavaTracker 是一套 JAVA CPG 上下游节点分析可视化工具：

- 面向大型 Java 项目保持较高索引与查询性能

- 重点展示类、方法、字段及方法体内部节点的上下游关系

- 提供更清晰、更友好的图谱与源码联动界面

  ![](images\display.png)

## 快速开始

### 1. 启动

```bash
cd /data/workspace/JavaTracker
go run ./cmd/javatracker -addr :8090 -root /path/to/java-project
```

启动后访问：

```text
http://127.0.0.1:8090
```

如果不传 `-root`，也可以在页面里输入扫描路径后再点击“开始索引”。

### 1.1 WebUI 上传 Java 代码

页面顶部支持两种方式：

- `索引目录`：输入已有 Java 项目路径并索引
- `上传 Java 文件`：直接上传一个或多个 `.java` 文件
- `上传源码目录`：选择一个本地 Java 或 Maven 源码目录
- `上传 ZIP 项目包`：上传打包后的 Java/Maven 项目源码压缩包
- `粘贴源码索引`：直接在页面中粘贴 Java 源码

说明：

- `上传 Java 文件` 和 `粘贴源码索引` 是最稳定的方式
- Maven 项目建议优先使用 `上传 ZIP 项目包`
- `上传源码目录` 依赖浏览器对 `webkitdirectory` 的支持，建议使用 Chromium/Chrome/Edge
- Maven 项目上传时，`pom.xml`、`src/main/java/`、`src/test/java/` 等结构会一并保存

上传后的源码会保存到：

```text
/data/workspace/JavaTracker/uploads/<timestamp>/
```

然后自动触发索引并展示结果。

### 2. 构建二进制

```bash
cd /data/workspace/JavaTracker
go build -o javatracker ./cmd/javatracker
./javatracker -addr :8090 -root /path/to/java-project
```

## 主要能力

- 项目索引
  - 并发扫描 `.java` 文件
  - 提取 package、import、class、interface、enum、field、method
  - 预构建调用边、字段访问边、继承/实现边、成员包含边

- 图谱查询
  - 支持 `upstream`、`downstream`、`both`
  - 支持深度和节点上限控制
  - 支持是否展开方法体内部节点

- 节点详情
  - 基本属性
  - 文件位置
  - 源码片段

## API

- `GET /api/status`
- `POST /api/index`
  - JSON: `{ "root": "/path/to/project" }`
- `GET /api/search?q=Controller&limit=50`
- `POST /api/upload`
  - multipart form: `files=<uploaded project files or zip>`
- `POST /api/snippet`
  - JSON: `{ "file_name": "Demo.java", "code": "public class Demo {}" }`
- `GET /api/graph?node=<nodeID>&direction=both&depth=2&limit=180&include_body=true`
- `GET /api/node?id=<nodeID>`

## 目录

```text
JavaTracker/
├── cmd/javatracker/          # 程序入口
├── internal/javatracker/     # 索引、查询、HTTP 服务
├── web/                      # 内嵌静态前端
└── README.md
```

