# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `aliyun-media-cli list`：按精确 prefix 使用 ListObjectsV2 分页返回对象元数据和不透明 `nextCursor`
- `aliyun-media-cli delete`：以稳定 JSON 结果幂等删除单个精确 object key
- GitHub Actions 最小质量门禁：test、race、vet 和双 CLI build；第三方 Action 固定到完整 commit SHA

### Changed
- upload/resolve/stat/list/delete 共用保守的 object key 输入安全子集
- list/delete 的库层非法输入统一返回 `INVALID_ARGUMENT`
- 两个 CLI 入口将 SIGINT/SIGTERM 转为 Cobra context；所有 OSS 网络操作均接入调用方 context
- STS 显式配置 5 秒连接超时和 15 秒读取超时，避免不可达或挂起端点导致命令无界等待

### Fixed
- `stat` 改用 OSS 详细元数据接口，确保已设置的 `Content-Type` 能按契约返回

### Security
- list 的 STS policy 仅允许目标 bucket 的 `oss:ListObjects`，并以 `StringEquals oss:Prefix` 收敛到本次 prefix
- delete 的 STS policy 仅允许目标 key 的 `oss:DeleteObject`；policy 使用结构化 JSON 序列化
- list/delete 将 SDK、HTTP 与云端响应错误收敛为稳定错误码，异常响应 fail closed
- list 对服务端回显的 continuation token 做 URL 编码兼容的一致性校验

## [0.1.0] - 2026-08-08

首发版本。两个无状态的 Go CLI,把阿里云 OSS 资源读写和身份证 OCR+核验从宿主应用里解耦出来。

### Added
- **`aliyun-media-cli`** — 阿里云 OSS 工具,子命令:
  - `profile add/list/remove/show` — 管理 OSS 凭证 profile(TOML,0600 权限,不进 git)
  - `upload` — 上传对象(公开/私密),每次操作走 STS AssumeRole,权限收敛到单对象单动作
  - `resolve` — 为私密对象生成短时签名 GET URL(可配 TTL)
  - `stat` — 查询对象元信息(存在性/大小/Content-Type/ETag)
- **`aliyun-idcard-cli`** — 阿里云 CloudAuth 工具,子命令:
  - `profile add/list/remove/show` — 管理 CloudAuth 凭证 profile
  - `verify` — 身份证 OCR + 二要素核验(人脸面/国徽面/姓名/身份证号),解析权威机构/有效期等字段
- JSON stdout / 退出码语义,适合脚本、CI、Agent 编排调用
- HMAC-SHA1 签名(percentEncode)与 CloudAuth 响应解析,覆盖率 ≥85%
- `task release` 交叉编译 linux/mac × amd64/arm64 + sha256 checksums

### Changed
- OSS region 现在为**必填**:profile 里 region 为空时直接报错,不再静默默认到特定 region(避免请求路由到错误 endpoint)

### Security
- 凭证(`profiles.local/*.toml`)、真实测试资源(`test-assets/`)全程 gitignored,仓库只跟踪 `*.example` 模板
- 强制 HTTPS endpoint,STS token 与私密媒体不走明文

[Unreleased]: https://github.com/ai-daming/aliyun-cli-toolkit/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/ai-daming/aliyun-cli-toolkit/releases/tag/v0.1.0
