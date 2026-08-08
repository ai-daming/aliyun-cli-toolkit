# aliyun-cli-toolkit — 设计文档

> 状态：草案 / 待评审
> 日期：2026-08-08

## 一句话

两个无状态的 Go CLI，把"上传/读取阿里云 OSS 资源"和"阿里云身份证 OCR+核验"从任何宿主应用里解耦出来。任何脚本、服务、Agent 都能用同一套工具操作资源，而不必各自集成云 SDK、各自持有凭证逻辑。

## 为什么做这个

Mamamate（一个月嫂/妈妈服务平台）的 Java server 现在把阿里云 OSS 和 CloudAuth 的逻辑直接编进进程里。结果是：

- **私密资源的读取逻辑散乱**：同一个"把存储引用变成可读 URL"的事，散落在 4 个类里、要处理 4 种不同的引用表示（`provider-media-asset:{id}`、`provider-private-media/` key、`resume-attachments/` key、裸 HTTP URL）。最近一连串 bug（#164/#165/#167）的根因都是"这个引用到底是哪种？我得分支判断"。
- **凭证逻辑绑死在应用里**：任何想操作资源的新调用方（内部 CLI、飞书 Agent、未来的 AI agent）要么重写一遍 OSS 集成，要么借道 server。
- **无法跨项目复用**：资源操作能力和业务应用耦合，换个项目又得来一遍。

我们把这两块能力抽成独立的、无状态的 CLI。Mamamate 变成它们的消费者之一；其它任何场景也能直接用。

## 设计原则

1. **无状态。** 每次调用是独立的：读 profile → 计算 → 输出 → 退出。不保活、不缓存连接、不依赖外部存储。跑完即走。
2. **单二进制。** Go 编译产物，零运行时依赖。`scp` + `chmod +x` 即用。交叉编译覆盖 linux/mac/windows × amd64/arm64。
3. **JSON in / JSON out。** stdout 永远是机器可读的 JSON；人读信息走 stderr。退出码：0 成功，非零失败（失败时 stderr 也给 JSON error）。
4. **一个云厂商，一套工具。** 不做跨云抽象——每个云的 OSS/身份核验 API 都不一样，硬抽象是过度设计。本项目绑死阿里云，命名也明示（`aliyun-*`）。要支持别的云，另起项目。
5. **key 是唯一的资源标识。** OSS object key 就是对象的身份，明文、可读、跨系统通用。CLI 不在 key 之上再套一层"资产引用"或"不透明 handle"——后者正是 Mamamate 现状的复杂度根源。
6. **私密对象从不返回稳定 URL。** 只有显式 `resolve` 能换出短时签名 URL（默认 15 分钟）。公开对象可以直接给稳定 URL。这是安全底线。
7. **Unix 组合优先。** 每个工具做一件事；工具之间通过管道/编排协作，不在内部互相 shell out。

## 范围

### 两个 CLI

| | `aliyun-media-cli` | `aliyun-idcard-cli` |
|---|---|---|
| 绑定 | 阿里云 OSS | 阿里云 CloudAuth / OCR |
| 能力 | upload / resolve / stat | verify（OCR + 二要素一体化） |
| 凭证 | OSS AK/SK + roleArn（运行时 AssumeRole） | CloudAuth AK/SK（独立一套） |
| 依赖 | 无 | 无（只吃 URL，不依赖 media-cli） |

两者的协作是**调用方编排**，不是内部 shell out。核验一张私密身份证图的标准编排：

```bash
FRONT=$(aliyun-media-cli resolve --profile mamamate --key .../front.jpg --ttl 5m | jq -r .url)
BACK=$(aliyun-media-cli resolve  --profile mamamate --key .../back.jpg  --ttl 5m | jq -r .url)
aliyun-idcard-cli verify --profile production --front-url "$FRONT" --back-url "$BACK"
```

idcard-cli 因此可以独立核验**任何来源**的身份证图 URL，不限于 OSS。

### 明确不做（YAGNI）

- **不做常驻 HTTP 服务。** 第一版纯 CLI（fork-exec 调用）。如果未来出现高频调用导致进程启动开销成为瓶颈，再加 `--serve` HTTP 模式——CLI 接口不变。
- **不做跨云抽象。**
- **不做 Mamamate 业务语义。** 不知道"月嫂"、"租户"、"provider-media-asset"这些概念。key 就是 key，上传就是上传。业务语义留给宿主应用。
- **不做 key 自动生成。** key 由调用方决定。CLI 可以提供 `--key-prefix` 便利参数，但不替调用方做业务命名决策。
- **不做 presign-put 和确认（confirm）环节。** Mamamate 现有 presign→PUT→confirm 三步，是因为浏览器直传场景需要。CLI 模型里调用方直接用 `upload` 一步传完；需要查对象是否到位用 `stat`。若将来真有"大文件不想进 CLI 内存"或"浏览器直传"的场景，再补 `presign-put`——届时 `stat` 已经在了，编排自然成立。

## `aliyun-media-cli` 契约

### profile 管理

凭证不进命令行参数（避免进 shell history / 进程列表）。register 时交互输入或从 env 读，存到本地配置文件。

```bash
# 注册一套凭证
aliyun-media-cli profile add mamamate \
  --bucket mamamate \
  --region cn-huhehaote \
  --endpoint oss-cn-huhehaote.aliyuncs.com \
  --role-arn acs:ram::YOUR_ACCOUNT_ID:role/oss-sts-role \
  --access-key-id "${OSS_AK}" \          # 也可省略，交互输入
  --access-key-secret "${OSS_SK}" \      # 同上
  --public-domain oss.mamamate.cn        # 可选；公开对象的稳定 CDN/自定义域名

aliyun-media-cli profile list
aliyun-media-cli profile remove mamamate
aliyun-media-cli profile show mamamate      # 脱敏显示（不输出 secret）
```

**没有默认 profile。** `--profile` 是所有操作命令的必填项，CLI 不会"记住"上次用的 profile。理由：一个工具管多套凭证时，默认值会让操作者在本意是生产、实际是测试的 profile 上误操作私密数据。显式传 `--profile` 把"用哪套凭证"变成每次清醒的决定。

配置存 `~/.config/aliyun-media-cli/profiles.toml`（权限 0600）。支持 `ALIYUN_MEDIA_CLI_HOME` 环境变量覆盖配置目录（便于 CI/容器）。

### 子命令：upload

CLI 直接把文件字节传到 OSS。适合中小文件（< 50MB 量级）。

```bash
aliyun-media-cli upload \
  --profile mamamate \
  --key provider-media/2026/08/discharge.jpg \
  --file ./discharge-summary.jpg \
  --content-type image/jpeg \
  [--private]                # 省略 = public
```

stdout（public）：
```json
{
  "key": "provider-media/2026/08/discharge.jpg",
  "visibility": "public",
  "url": "https://oss.mamamate.cn/provider-media/2026/08/discharge.jpg",
  "size": 234567,
  "etag": "d41d8cd98f00b204e9800998ecf8427e"
}
```

stdout（private）——注意 `url` 字段缺省：
```json
{
  "key": "provider-private-media/.../x.jpg",
  "visibility": "private",
  "size": 89012,
  "etag": "d41d8cd98f00b204e9800998ecf8427e"
}
```

`etag` 字段说明：对应 OSS 对象的 `ETag`。对单次 PUT 的小文件，**ETag = 内容 MD5**，调用方可本地算 MD5 与之比对，做上传完整性校验（这正是 mm-resume 现在 `confirm`+sha256 在做的事，这里用 ETag 更直接）。本 CLI 的 `upload` 走单 PUT，etag 即 MD5，可靠。

### 子命令：resolve

**最关键的子命令**——把一个 key 变成可读 URL。对调用方透明：不关心对象是公是私，给 key 出 URL。这是本次重构最大的价值收口点。

```bash
# 私密对象：现签短时 URL
aliyun-media-cli resolve --profile mamamate --key provider-private-media/.../x.jpg --ttl 15m
```
```json
{
  "url": "https://mamamate.oss-cn-huhehaote.aliyuncs.com/...signed...",
  "visibility": "private",
  "expiresAt": "2026-08-08T17:45:00Z"
}
```

```bash
# 公开对象：直接给稳定 URL
aliyun-media-cli resolve --profile mamamate --key provider-media/.../x.jpg
```
```json
{
  "url": "https://oss.mamamate.cn/provider-media/.../x.jpg",
  "visibility": "public"
}
```

公私判断：**调用方显式声明**，CLI 不猜。`resolve` 默认按私密处理（现签短时 URL）——这是更安全的默认；加 `--public` 则直接返回稳定 URL（不校验对象是否真的公开，因为公开对象的 ACL 就是公开，无需查）。设计理由：HEAD 读 ACL 是一次额外网络往返，而调用方在 upload 时就知道自己传的是公是私，这个信息不该丢；让 CLI 去"探测"ACL 既慢又引入新的失败模式。

resolve **只返回 URL，不返回 etag/size 等元信息**。它的职责是把 key 变成可读 URL，一件事；要对象元信息走 `stat`。混进去会让 resolve 对公开对象也得多一次 HEAD，破坏"纯转换"的语义。

### 子命令：stat

查对象是否存在及元信息，不下载内容。对应 Mamamate `OssStsService.objectContentLength`。

```bash
aliyun-media-cli stat --profile mamamate --key some/key
```
```json
{"exists": true, "size": 234567, "contentType": "image/jpeg", "etag": "d41d8cd98f00b204e9800998ecf8427e"}
```
```json
{"exists": false}
```

## `aliyun-idcard-cli` 契约

### profile 管理

```bash
aliyun-idcard-cli profile add production \
  --region cn-shanghai \
  --endpoint cloudauth.cn-shanghai.aliyuncs.com \
  --access-key-id "${VERIFY_AK}" \
  --access-key-secret "${VERIFY_SK}"

aliyun-idcard-cli profile list
aliyun-idcard-cli profile remove production
aliyun-idcard-cli profile show production
```

与 media-cli 一致：`--profile` 必填，没有默认 profile。

凭证独立于 media-cli——CloudAuth 的 AK/SK 不是 OSS 那套。这与 Mamamate 现状一致（`aliyun.verify.*` vs `oss.sts.*`）。

### 子命令：verify

OCR + 二要素一体化核验。对应阿里云 `Id2MetaVerifyWithOCR` API，对应 Mamamate `AliyunId2MetaVerifyService`。

```bash
aliyun-idcard-cli verify \
  --profile production \
  --front-url "https://...signed.../front.jpg" \
  [--back-url  "https://...signed.../back.jpg"]
```
```json
{
  "ok": true,
  "passed": true,
  "name": "张三",
  "idCard": "110101199001011234",
  "gender": "男",
  "ethnicity": "汉",
  "birthDate": "1990-01-01",
  "address": "...",
  "requestId": "...",
  "raw": { "BizCode": "1" }
}
```

**只吃 URL，不吃 key。** 调用方负责先把私密 key resolve 成可读 URL（见上文"调用方编排"）。这保证 idcard-cli 零依赖 media-cli，可以核验任何来源的身份证图。

失败语义：
- API 调用本身失败（网络、鉴权）→ `ok: false`，非零退出码，stderr JSON 带 error。
- API 调用成功但核验不一致（BizCode != 1）→ `ok: true, passed: false`，**退出码 0**（这不是 CLI 的错，是业务结果）。

## 仓库结构

单仓多模块起步，未来需要独立发版再拆。

```
aliyun-cli-toolkit/
├── docs/
│   └── DESIGN.md              # 本文件
├── cmd/
│   ├── aliyun-media-cli/      # media CLI 入口 main.go
│   └── aliyun-idcard-cli/     # idcard CLI 入口 main.go
├── internal/
│   ├── media/                 # OSS 操作实现（upload/resolve/stat）
│   ├── idcard/                # CloudAuth 核验实现
│   ├── profile/               # 共享：profile 管理（add/list/default/remove）
│   ├── osssign/               # 共享：OSS 签名/STS 工具（如复用）
│   └── cli/                   # 共享：JSON 输出、错误退出码、cobra 公共配置
├── go.mod
├── go.sum
├── README.md
├── Taskfile.yml               # 构建/交叉编译（与 mm-resume 一致）
└── .github/workflows/         # CI：test + release（build 二进制）
```

两个 CLI 的业务逻辑（`internal/media`、`internal/idcard`）互相独立；共享代码（profile 管理、CLI 基础设施、JSON 输出约定）放 `internal/` 公共包。

## 技术选型

- **语言**：Go（1.22+）。理由：单静态二进制、官方阿里云 SDK、CLI 生态成熟（cobra/viper）、开源贡献门槛低、与 mm-resume 技术栈一致。
- **CLI 框架**：spf13/cobra + spf13/viper（配置）。
- **OSS SDK**：`github.com/aliyun/aliyun-oss-go-sdk/oss`（官方）。
- **STS**：`github.com/alibabacloud-go/sts-20150401`（官方，与 Mamamate Java 用的同源）。
- **CloudAuth**：直接 HTTP + 手写签名（与 Mamamate Java 现状一致，`AliyunId2MetaVerifyService` 就是这么做的；签名逻辑短，不引入额外 SDK）。
- **配置**：TOML（`github.com/BurntSushi/toml` 或 `github.com/pelletier/go-toml/v2`），文件权限 0600。
- **构建**：Taskfile（与 mm-resume 一致），交叉编译 linux/mac/windows × amd64/arm64。

## 安全姿态

- 凭证存本地配置文件，权限 0600，不进 git，不在命令行明文传。
- 私密对象只通过短时签名 URL 暴露（默认 15 分钟 TTL，可调但有上限）。
- STS AssumeRole 用最小权限 policy（对应 Mamamate 现有的 `buildObjectPolicy`：单对象、单 action）。
- idcard 核验结果里的 `idCard` 字段是明文身份证号——调用方有责任妥善处理；CLI 不在本地留存核验结果。
- profile 文件路径支持 `ALIYUN_MEDIA_CLI_HOME` / `ALIYUN_IDCARD_CLI_HOME` 覆盖，便于容器场景挂载 secret。

## Mamamate 改造路径（消费者侧，不在本项目内）

只讲路径，不改 Mamamate 代码。渐进、可停在任意中间态：

1. **集成验证**：在 Mamamate 机器上 register 好 profile，手动跑通 upload/resolve/verify。
2. **新增 MediaCliGateway**：Java 内 fork-exec CLI 的薄封装。新代码走它，老代码不动。
3. **逐个替换 resolver**：把 4 个私有读 resolver（`ProviderMediaAssetService.resolveConfirmedPrivateMedia`、`ProviderPrivateAttachmentService`、`IdCardImageReferenceResolver`、`ResumeLegacyIdCardImageResolver`）换成 `MediaCliGateway.resolve()`。每换一个跑测试。
4. **替换 OCR 调用**：`IdCardVerificationService.performOcr` 改成 resolve + fork-exec idcard-cli。
5. **退役旧代码**：删除 `OssStsService`、`AliyunId2MetaVerifyService`、`AliyunOcrService`、`AliyunVerifyService`，把 env var 里的 AK/SK 移到 CLI profile。

## 开源化考量

- **许可证**：MIT（最宽松，利于采纳）。
- **命名**：`aliyun-cli-toolkit`。如果未来阿里云有意见再改。
- **README**：中英双语，开宗明义"无状态 CLI、单二进制、JSON I/O"。
- **示例**：放 `examples/` 下，覆盖：简单上传、私密读、身份证核验编排、CI 场景、飞书 Agent 场景。
- **CI**：GitHub Actions，每个 PR 跑 test + vet + 跨平台 build；release 自动产出各平台二进制。
- **语义化版本**：v0.x 期间契约可能变；v1.0 锁定 JSON schema。

## 待定 / 实现阶段再决

- `resolve` 对不存在的 key 的行为（报错 vs 返回 exists:false）——倾向报错 + 非零退出码，因为 resolve 的语义是"给我这个对象的 URL"，对象不存在是异常。
- 是否需要 `cp`（对象复制）、`rm`（对象删除）、`ls`（列举）子命令——先不做，YAGNI。
- STS 凭证在单次 CLI 进程内的复用（一个进程内多次操作可共享一次 AssumeRole 的结果）——实现细节，不暴露给外部。
