from PIL import Image, ImageFilter, ImageOps
import os
src = r"C:\Users\96495\AppData\Local\Temp\codex-clipboard-4cfa0980-99a1-4dd3-b6c7-77c30a38f5e7.png"
out = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\ref2"
os.makedirs(out, exist_ok=True)
im = Image.open(src).convert("L")
print("size", im.size)
# 放大 + 对比度拉伸 + 锐化，便于逐块阅读
big = im.resize((im.width*8, im.height*8), Image.LANCZOS)
big = ImageOps.autocontrast(big, cutoff=1)
big = big.filter(ImageFilter.UnsharpMask(radius=6, percent=180, threshold=2))
big.save(os.path.join(out, "full_big.png"))
print("full_big", big.size)
# 逐行切块（按参考图的四层位置）
w,h = big.size
blocks = {
  "row1_top.png": (0, 0, w, int(h*0.16)),
  "row1_top_right.png": (int(w*0.6), 0, w, int(h*0.16)),
  "row2_mid.png": (0, int(h*0.11), int(w*0.75), int(h*0.28)),
  "row2_mid_right.png": (int(w*0.55), int(h*0.11), w, int(h*0.30)),
  "row3_bottom_left.png": (0, int(h*0.34), int(w*0.6), h),
  "row3_bottom_right.png": (int(w*0.6), int(h*0.34), w, h),
}
for name, box in blocks.items():
    c = big.crop(box)
    p = os.path.join(out, name); c.save(p); print(p, c.size)
