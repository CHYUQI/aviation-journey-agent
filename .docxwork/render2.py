import pymupdf, os
pdf = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\04.pdf"
out = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\pages"
doc = pymupdf.open(pdf)
for i in range(9, 16):
    pix = doc[i].get_pixmap(dpi=100)
    f = os.path.join(out, f'p{i+1:02d}.png')
    pix.save(f); print(f)
