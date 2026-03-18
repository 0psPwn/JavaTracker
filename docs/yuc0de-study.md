# YuC0de-main CPG 上下游功能学习记录

## 观察到的核心链路

原项目关键文件：

- `/data/workspace/YuC0de-main/backend/cpg/handler.go`
- `/data/workspace/YuC0de-main/backend/cpg/code_parser.go`
- `/data/workspace/YuC0de-main/backend/engine/indexer.go`
- `/data/workspace/YuC0de-main/backend/engine/java_parser.go`
- `/data/workspace/YuC0de-main/frontend/src/views/CPGAnalysis.vue`

其工作方式大致如下：

1. 后端扫描 Java 项目并建立 `ClassMap / MethodMap / FieldMap / CallerMap / ExtendsMap`
2. 前端左侧列出类、方法、字段节点
3. 用户选中节点后，请求 `GetGraphHandler`
4. 后端以该节点为起点执行 BFS
5. 在 BFS 中按类型扩展：
   - 方法节点：父类、调用下游、调用上游、字段访问、方法体内部结构
   - 字段节点：父类、被方法访问
   - 类节点：继承上下游、成员方法、成员字段
6. 前端用 `vis-network` 做统一力导布局

## 原实现优点

- 不依赖编译，直接对源码做轻量解析
- 数据结构简单，便于快速实现类/方法/字段级关系展示
- 方法体内部补充 AST/CFG/PDG 风格节点，直观性较好

## 原实现局限

### 1. 全局状态模型不利于大型项目和并发使用

- 使用 `CurrentTaskID / CurrentIndex / IndexMutex` 维护单份全局索引
- 本质上更适合单任务、低并发场景

### 2. 查询阶段做了过多即时推导

- 点击节点时才 BFS 扩展
- 方法调用解析依赖类内、父类、import、全局按名字兜底检索
- 大项目中容易引入误连边和额外查询成本

### 3. 前端图布局在大图场景容易拥挤

- `vis-network` 力导图对几十个点尚可
- 节点和边稍多后，语义分层不明显，阅读成本高

### 4. 方法体节点生成偏“全量”

- 选中方法后即把内部变量、表达式、控制流、返回节点都加进去
- 当方法较长、深度较大时会迅速放大图规模

## JavaTracker 的设计取舍

基于上述分析，JavaTracker 采用以下策略：

1. 项目级预索引
   - 预先构建类、方法、字段、调用、访问、继承边
   - 查询时只做局部截取与组合

2. 方法体节点惰性生成
   - 只在图查询时针对已选中的方法生成内部节点
   - 避免对整仓库预建巨量语句节点

3. 更明确的布局分层
   - 上游、焦点、下游、方法体四个视觉泳道
   - 大图时结构比纯力导布局更稳定

4. 单二进制嵌入式部署
   - Go HTTP 服务直接内嵌前端
   - 启动方式简单，便于交付与复现
