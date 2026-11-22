# dash 输出跨后端核对工具

该目录包含用于核对 pdf / png / svg 三个后端虚线渲染一致性的独立工具，
不参与 `go test ./...`（Go 工具链会忽略 `testdata` 目录）。

## 结构

- `cmd/dashverify/main.go`：用同一组图形（水平线、贝塞尔曲线、多子路径、
  Save/Restore、矩阵缩放、线帽/连接）分别生成 `.pdf`、`.png`、`.svg`。
- `script/verify.py`：用 pdfium 光栅化 PDF、cairosvg 光栅化 SVG，并将 PNG
  合成到白底，逐段比较可见区间、offset 和缩放后的 dash 长度。

## 运行

```sh
go run ./testdata/dashverify/cmd/dashverify/ /tmp/dashout
python -I testdata/dashverify/script/verify.py /tmp/dashout
```

Python 解释器需要 `pypdfium2`、`pillow`、`cairosvg` 三个包。

## 已知后端差异

图像后端（draw2dimg）的 stroker 不按线帽延伸 dash 端点，而 PDF/SVG 规范
要求 round/square cap 各延伸半个线宽。脚本对线帽场景以 SVG 渲染为规范基准
核对 PDF，并单独检查 PNG 的 dash 中心与间距是否仍然对齐。
