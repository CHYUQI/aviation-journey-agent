import pymupdf, os
pdf = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\04.pdf"
out = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\pages"
os.makedirs(out, exist_ok=True)
doc = pymupdf.open(pdf)
print('pages', doc.page_count)
for i in range(0, min(9, doc.page_count)):
    p = doc[i]
    pix = p.get_pixmap(dpi=100)
    f = os.path.join(out, f'p{i+1:02d}.png')
    pix.save(f)
    print(f, pix.width, pix.height)
