from PIL import Image
import os, glob
base = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\out04\images"
for p in sorted(glob.glob(os.path.join(base,'*.png'))):
    im = Image.open(p)
    print(os.path.basename(p), im.size)
