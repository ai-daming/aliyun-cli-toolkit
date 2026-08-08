# aliyun-cli-toolkit

两个无状态的 Go CLI，把"上传/读取阿里云 OSS 资源"和"阿里云身份证 OCR+核验"从任何宿主应用里解耦出来。

- **`aliyun-media-cli`** — 阿里云 OSS：upload / presign-put / resolve / stat
- **`aliyun-idcard-cli`** — 阿里云 CloudAuth：身份证 OCR + 二要素核验

## 特点

- **无状态**：每次调用独立，读 profile → 计算 → 输出 → 退出。不保活。
- **单二进制**：Go 编译，零运行时依赖。拷过去就能跑。
- **JSON I/O**：stdout 永远是机器可读 JSON；退出码标识成败。适合脚本、CI、Agent 调用。
- **Unix 组合**：两个 CLI 互不依赖，通过管道/编排协作。

## 状态

🚧 草案设计阶段。设计文档见 [docs/DESIGN.md](docs/DESIGN.md)。

## License

MIT
