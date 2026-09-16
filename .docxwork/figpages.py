import pymupdf, os, re
pdf = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\04.pdf"
out = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\pages"
doc = pymupdf.open(pdf)
for i in range(25, doc.page_count):
    pix = doc[i].get_pixmap(dpi=80)
    f = os.path.join(out, f'p{i+1:02d}.png'); pix.save(f)
pat = re.compile(r"图\s?([2345])-(\d)")
for i in range(doc.page_count):
    t = doc[i].get_text()
    caps = sorted(set(m.group(0) for m in pat.finditer(t)))
    if caps:
        print(f"page {i+1} (body {i-3}):", caps)
