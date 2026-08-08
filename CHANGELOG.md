# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
