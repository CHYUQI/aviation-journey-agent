import zipfile, os
p = r"D:\VST DEVELOP\aviation-journey-agent\docs\基于智能Agent的民航旅客起飞前行程规划与延误风险预警系统_课程汇报PPT(1).pptx"
out = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\ppt_media"
os.makedirs(out, exist_ok=True)
z = zipfile.ZipFile(p)
media = [n for n in z.namelist() if n.startswith("ppt/media/")]
print("media files:", len(media))
from PIL import Image
import io
rows = []
for n in media:
    data = z.read(n)
    try:
        im = Image.open(io.BytesIO(data))
        rows.append((n, im.size, im.format, len(data)))
    except Exception as e:
        rows.append((n, None, None, len(data)))
for r in sorted(rows, key=lambda r: -(r[1][0]*r[1][1] if r[1] else 0))[:40]:
    print(r)
