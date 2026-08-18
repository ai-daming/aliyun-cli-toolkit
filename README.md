# aliyun-cli-toolkit

两个无状态的 Go CLI，把"上传/读取阿里云 OSS 资源"和"阿里云身份证 OCR+核验"从任何宿主应用里解耦出来。

- **`aliyun-media-cli`** — 阿里云 OSS：upload / resolve / stat / list / delete
- **`aliyun-idcard-cli`** — 阿里云 CloudAuth：身份证 OCR + 二要素核验

## 特点

- **无状态**：每次调用独立，读 profile → 计算 → 输出 → 退出。不保活。
- **单二进制**：Go 编译，零运行时依赖。拷过去就能跑。
- **JSON I/O**：stdout 永远是机器可读 JSON；退出码标识成败。适合脚本、CI、Agent 调用。
- **Unix 组合**：两个 CLI 互不依赖，通过管道/编排协作。

## 快速开始

```bash
# 构建
task build
# 产出 bin/aliyun-media-cli 和 bin/aliyun-idcard-cli

# 注册凭证（不进 git，存 ~/.config 或 $ALIYUN_MEDIA_CLI_HOME）
aliyun-media-cli profile add mamamate \
  --bucket mamamate --region cn-huhehaote \
  --endpoint oss-cn-huhehaote.aliyuncs.com \
  --role-arn acs:ram::...:role/... \
  --access-key-id ... --access-key-secret ... \
  --public-domain oss.mamamate.cn

# 上传（公开）
aliyun-media-cli upload --profile mamamate \
  --key media/photo.jpg --file ./photo.jpg --content-type image/jpeg
→ {"key":"media/photo.jpg","url":"https://...","visibility":"public","size":12345,"etag":"..."}

# 上传（私密，不返回 url）
aliyun-media-cli upload --profile mamamate \
  --key private/doc.jpg --file ./doc.jpg --private
→ {"key":"private/doc.jpg","visibility":"private","size":12345,"etag":"..."}

# 读取私密对象 → 短时签名 URL
aliyun-media-cli resolve --profile mamamate --key private/doc.jpg --ttl 15m
→ {"url":"https://...signed...","visibility":"private","expiresAt":"..."}

# 按精确 prefix 分页列举；nextCursor 原样传给下一页，null 表示结束
aliyun-media-cli list --profile mamamate --prefix media/2026- --limit 100
→ {"objects":[{"key":"media/2026-08/photo.jpg","lastModified":"2026-08-18T10:00:00Z","size":12345,"etag":"..."}],"nextCursor":null}

# 幂等删除一个精确 key；对象原本不存在时也返回相同成功结果
aliyun-media-cli delete --profile mamamate --key media/2026-08/photo.jpg
→ {"key":"media/2026-08/photo.jpg","deleted":true}

# 身份证核验（先 resolve 成可读 URL，再 verify —— 调用方编排）
FRONT=$(aliyun-media-cli resolve --profile mamamate --key .../front.jpg --ttl 5m | jq -r .url)
aliyun-idcard-cli verify --profile verify --front-url "$FRONT"
→ {"ok":true,"passed":true,"name":"张三","idCard":"...","requestId":"..."}
```

## 测试

默认测试不需要真实凭证，也不会访问或删除真实 OSS 对象。list/delete 的协议测试使用本地 HTTP server 驱动真实 STS/OSS SDK，校验请求、最小权限 policy、分页和异常响应；已有云端集成测试仅在显式提供 `profiles.local/` 凭证时运行。

```bash
task test-cover   # 跑测试并报告仓库覆盖率；变更区域门槛 ≥85%
```

## 状态

✅ `aliyun-media-cli` — 完成（upload/resolve/stat/list/delete/profile）
✅ `aliyun-idcard-cli` — 完成（verify/profile）

## 设计文档

- [docs/DESIGN.md](docs/DESIGN.md) — 架构设计
- [docs/plans/](docs/plans/) — 实现计划
- [CHANGELOG.md](CHANGELOG.md) — 版本变更

## License

Apache-2.0,详见 [LICENSE](LICENSE)。
