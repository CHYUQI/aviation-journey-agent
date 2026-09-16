from PIL import Image
import numpy as np, os
base = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\out04\images"
display = {  # cm sizes from docx
 '04_fig002.png': (8.26,1.77), '04_fig003.png': (14.22,2.21), '04_fig004.png': (14.22,2.54),
 '04_fig005.png': (14.22,3.54), '04_fig006.png': (14.22,3.58), '04_fig007.png': (14.22,13.68),
 '04_fig008.png': (14.22,12.04), '04_fig009.png': (14.22,11.5), '04_fig010.png': (13.84,13.31),
 '04_fig011.png': (14.10,12.75), '04_fig012.png': (13.97,9.37), '04_fig013.png': (12.95,12.29),
 '04_fig014.png': (10.16,10.28), '04_fig015.png': (14.22,4.67), '04_fig016.png': (14.22,6.18),
}
def text_row_height(path, x0f, x1f, y0f, y1f):
    im = Image.open(path).convert('L')
    a = np.array(im)
    h, w = a.shape
    sub = a[int(h*y0f):int(h*y1f), int(w*x0f):int(w*x1f)]
    dark = (sub < 120).sum(axis=1)
    rows = np.where(dark > 2)[0]
    if len(rows) == 0: return None
    return rows.max()-rows.min()+1
for name,(cw,ch) in display.items():
    p = os.path.join(base, name)
    im = Image.open(p); w,h = im.size
    cm_per_px = ch / h
    # sample a body-text band
    th = text_row_height(p, 0.05, 0.5, 0.45, 0.75)
    if th is None: th = text_row_height(p, 0.05, 0.6, 0.2, 0.6)
    pt = th * cm_per_px * 28.3465 / 1.0  # 1cm = 28.3465pt
    print(f"{name}: {w}x{h}px -> {cw}x{ch}cm; sample glyph height {th}px -> {pt:.1f}pt")
