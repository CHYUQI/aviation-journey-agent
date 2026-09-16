import pymupdf, os
pdf = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\04.pdf"
out = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\pages"
doc = pymupdf.open(pdf)
for i in range(25, 30):
    pix = doc[i].get_pixmap(dpi=80)
    f = os.path.join(out, f'p{i+1:02d}.png')
    pix.save(f); print(f)
# also find which pages contain the figure captions by searching text
for i in range(doc.page_count):
    t = doc[i].get_text()
    hits = [s for s in ['图2-1','图3-1','图3-2','图3-3','图3-4','图4-1','图4-2','图4-3','图4-4','图4-5','图4-6','图5-1','图5-2','图5-3','图5-4'] if ('图 4-4' in t or s in t)]
