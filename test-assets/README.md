# test-assets/

测试用的真实资源文件。**此目录内容不进 git**（见根 `.gitignore`），每人本机自备。

放什么：

- 图片（jpg/png）：测 upload / resolve / stat，模拟"出院记录""出院小结""身份证照片"等场景
- 二进制（zip）：测非图片 content-type
- 文本（txt）：测小文件

建议每个 fixture 配一个稳定的 key 前缀便于复跑，例如：

```bash
aliyun-media-cli upload \
  --profile mamamate \
  --key test-assets/wechat-img-1.jpg \
  --file ./微信图片_20260715155830_16_5276.jpg \
  --content-type image/jpeg
```
