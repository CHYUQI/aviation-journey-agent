import zipfile, re
p = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\04.docx"
z = zipfile.ZipFile(p)
xml = z.read("word/document.xml").decode("utf-8")
print("len", len(xml))
instr = re.findall(r"<w:instrText[^>]*>([^<]*)</w:instrText>", xml)
print("instrText count", len(instr))
for s in instr[:12]:
    print(repr(s))
print("PAGEREF count", xml.count("PAGEREF"))
print("fldSimple count", xml.count("fldSimple"))
# count sdt (content controls, TOC)
print("sdt count", xml.count("<w:sdt>"))
# find drawings and their extents (cx,cy EMU) to compute displayed sizes
exts = re.findall(r'<wp:extent cx="(\d+)" cy="(\d+)"/>', xml)
print("drawings", len(exts))
for cx, cy in exts:
    print(round(int(cx)/360000,2), "cm x", round(int(cy)/360000,2), "cm")
