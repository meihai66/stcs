# 本地 GPT 生图

一个本地运行的生图工具,对接 **OpenAI 兼容的中转站**(填入 base URL + 密钥),使用 `gpt-image-2` 等模型生图,支持 2K / 4K 等分辨率。

灵感来自 [ZyphrZero/chatgpt2api](https://github.com/ZyphrZero/chatgpt2api),但用 Python + 轻量 Web UI 重写,聚焦「对接中转站 API」这一种方式。

## 功能

- 🖼 **文生图**:提示词 → 图片
- ✏️ **图生图 / 编辑**:上传参考图 + 提示词
- 🤖 **多模型**:gpt-image-2、gemini-2.5-flash-image(Nano Banana)等预置 + 自定义,每次生成可切换
- 🛒 **提示词市场**:内置 300+ 社区精选提示词(来自 banana-prompt-quicker),搜索/分类/一键套用
- 🔍 **参考图反推**:上传一张图,用视觉模型自动反推出提示词
- 🔁 **继续创作**:在历史结果上「继续」,把它当底图继续改图
- ⚡ **生图压测**:自定义总请求数 + 并发数,实测中转站成功率 / 吞吐(req/s)/ 延迟(平均、P50、P95)/ 错误分类(只测速不存图)
- 📐 **分辨率选择器**:画面比例(1:1 / 3:2 / 2:3 / 16:9 / 9:16)× 分辨率档位(1K / 2K / 4K)组合,也可自定义任意尺寸
- 🗂 **多提示词并行队列**:提示词框每行一条,各自作为独立任务**并行**生成;并发数可在设置里调(默认 3)。还可设「每条重复」批量出图,网页实时显示进度
- 📜 **历史记录**:SQLite 持久化(`data.db`),含提示词/参数/缩略图,可「再用此提示词」或删除
- ⭐ **提示词模板 / 收藏**:常用提示词一键保存、复用
- 🔀 **多套中转站配置**:保存多个中转站(地址/密钥/模型/格式),顶部下拉一键切换当前用哪个,不用每次重填
- ⚙️ **设置页**:在网页里填中转站地址和密钥,保存到本机 `config.json`(支持 `GPTIMG_CONFIG_PATH` 自定义位置)
- 🔌 **OpenAI 兼容 API**:对外提供 `/v1/images/generations`,可被其他程序调用
- 🖼 生成图片自动保存到 `outputs/`,网页可浏览下载

## 快速开始

**最简单:双击 `start.bat`**(首次会自动建虚拟环境、装依赖,然后启动并打开浏览器)。

或手动:

```powershell
# 1. 安装依赖(建议用虚拟环境)
pip install -r requirements.txt

# 2. 启动
python app.py
#   或:python -m uvicorn app:app --host 127.0.0.1 --port 5311

# 3. 打开浏览器
#    http://127.0.0.1:5311
#    首次会弹出「设置」,填入中转站地址、密钥、模型即可
```

## 配置

设置可在网页「⚙ 设置」里填写,也可手动复制 `config.example.json` 为 `config.json`:

| 字段 | 说明 |
|------|------|
| `base_url` | 中转站地址,如 `https://api.example.com`(不含 `/v1`) |
| `api_key` | 中转站密钥 `sk-...` |
| `model` | 生图模型,如 `gpt-image-2` |
| `default_size` | 默认尺寸,如 `1024x1024` |
| `default_quality` | 默认质量 `high/medium/low/auto` |
| `timeout` | 单次请求超时秒数,4K 建议 ≥300 |
| `concurrency` | 队列并发数,同时执行几个任务(1~16,默认 3,改后需重启) |
| `server_api_key` | 对外 API 的访问密钥,留空则不校验 |

也支持环境变量覆盖:`GPTIMG_BASE_URL`、`GPTIMG_API_KEY`、`GPTIMG_MODEL`、`GPTIMG_SERVER_API_KEY`。

## 作为 API 服务调用

启动后可像调用 OpenAI 一样调用本服务:

```bash
curl http://127.0.0.1:5311/v1/images/generations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <server_api_key 或任意值>" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "a cyberpunk city at night",
    "size": "2048x2048",
    "n": 1,
    "response_format": "b64_json"
  }'
```

## 说明

- 请求实际由本服务转发给你配置的中转站;尺寸 / 质量是否支持取决于中转站和模型。
- `config.json` 含密钥,已加入 `.gitignore`,不要提交。
- 仅监听 `127.0.0.1`(本机)。如需局域网访问,把启动 host 改为 `0.0.0.0` 并注意设置 `server_api_key`。
