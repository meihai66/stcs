# STCS · GPT/Gemini 生图压测台

对接 **OpenAI 兼容中转站** 的生图工具 + 中转站压测台。**Go 后端 + React(Vite + Tailwind)前端**,
单二进制部署、前端嵌入二进制、SQLite 无 CGO,镜像极小。进入测试页需输入访问密码。

> 由原 Python 版用 Go 全量重写。

## 功能

- 🖼 **文生图 / 对话生图 / 图片编辑**:三种请求格式(`images` / `chat` / `edits`)适配不同中转站
- 🤖 **多模型**:gpt-image-2、gemini-2.5-flash-image(Nano Banana)等预置 + 自定义,逐次可切
- ⚡ **生图压测**:自定义总请求数 + 并发数,实测成功率 / 吞吐(req/s)/ 延迟(平均、P50、P95)/ 错误分类,实时进度
- 🛒 **提示词市场**:内置社区精选提示词(banana-prompt-quicker),搜索 / 分类 / 一键套用
- 🔍 **参考图反推**:上传一张图,用视觉模型反推出提示词
- 🔁 **继续创作**:在历史结果上继续改图
- 📐 **分辨率选择器**:比例(1:1 / 3:2 / 2:3 / 16:9 / 9:16)× 档位(1K / 2K / 4K),也可自定义
- 🗂 **多提示词并行队列**:每行一条,各自独立任务并行;并发数可调;支持「每条重复」批量
- 📜 **历史记录**:SQLite 持久化,缩略图 / 再用 / 删除
- ⭐ **提示词模板 / 收藏**
- 🔀 **多套中转站配置**:保存多个,顶部下拉一键切换
- 🔌 **OpenAI 兼容 API**:对外 `/v1/images/generations`,可被其他程序调用
- 🔐 **访问密码门**:进测试页需输密码(环境变量配置)

## 快速部署(其他机器,推荐 Docker)

```bash
# 1. 拿到 docker-compose.yml(改下 STCS_PASSWORD)
# 2. 启动
docker compose up -d
# 3. 浏览器打开 http://<服务器IP>:5311 ,输入访问密码
```

或直接 docker run:

```bash
docker run -d --name stcs -p 5311:5311 \
  -e STCS_PASSWORD=你的密码 \
  -v $PWD/data:/app/data \
  meihai0211/stcs:latest
```

登录后点「⚙ 设置」,填中转站地址 / 密钥 / 模型即可开始。配置与图片持久化在 `./data` 卷。

## 环境变量

| 变量 | 说明 | 默认 |
|------|------|------|
| `STCS_PASSWORD` | **进入测试页的访问密码** | `admin888` |
| `STCS_PORT` | 监听端口 | `5311` |
| `STCS_DATA_DIR` | 数据目录(config.json / data.db / outputs) | `./data` |
| `STCS_BASE_URL` | (可选)预置中转站地址,覆盖配置文件 | — |
| `STCS_API_KEY` | (可选)预置中转站密钥 | — |
| `STCS_MODEL` | (可选)预置生图模型 | — |
| `STCS_SERVER_API_KEY` | (可选)对外 `/v1` API 的访问密钥,留空不校验 | — |

## 本地开发

```bash
# 后端(需要 Go 1.24+)
npm --prefix web install && npm --prefix web run build   # 先构建前端(供 go embed)
go run .                                                  # http://127.0.0.1:5311

# 前端热更新(另开终端,代理到 5311)
npm --prefix web run dev                                  # http://127.0.0.1:5312
```

## 作为 API 服务调用

```bash
curl http://<IP>:5311/v1/images/generations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <STCS_SERVER_API_KEY 或任意值>" \
  -d '{"model":"gpt-image-2","prompt":"a cyberpunk city","size":"2048x2048","n":1,"response_format":"b64_json"}'
```

## 发布(维护者)

```bash
bash scripts/release.sh   # 用 VERSION 构建并推送 镜像:<版本> 与 :latest,然后补丁号 +1
```

镜像仓库:`meihai0211/stcs`。源码仓库:`git@github.com:meihai66/stcs.git`。

## 技术栈

- 后端:Go 1.24,标准库 `net/http`(Go 1.22+ 路由),`modernc.org/sqlite`(纯 Go,无 CGO)
- 前端:React 18 + Vite 6 + Tailwind CSS v4 + lucide-react,构建产物 `//go:embed` 进二进制
- 镜像:多阶段构建(node → golang → alpine),静态二进制
