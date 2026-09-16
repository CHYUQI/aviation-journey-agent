from PIL import Image
import numpy as np, os, statistics
base = r"D:\VST DEVELOP\aviation-journey-agent\.docxwork\out04\images"
display = {
 '04_fig002.png': (8.26,1.77), '04_fig003.png': (14.22,2.21), '04_fig004.png': (14.22,2.54),
 '04_fig005.png': (14.22,3.54), '04_fig006.png': (14.22,3.58), '04_fig007.png': (14.22,13.68),
 '04_fig008.png': (14.22,12.04), '04_fig009.png': (14.22,11.5), '04_fig010.png': (13.84,13.31),
 '04_fig011.png': (14.10,12.75), '04_fig012.png': (13.97,9.37), '04_fig013.png': (12.95,12.29),
 '04_fig014.png': (10.16,10.28), '04_fig015.png': (14.22,4.67), '04_fig016.png': (14.22,6.18),
}
for name,(cw,ch) in display.items():
    p = os.path.join(base, name)
    im = Image.open(p).convert('L'); a = np.array(im); h,w = a.shape
    # restrict to a column band that contains text (avoid big empty areas)
    sub = a[:, int(w*0.04):int(w*0.55)]
    dark = (sub < 120).sum(axis=1)
    runs=[]; start=None
    for i,v in enumerate(dark):
        if v>1 and start is None: start=i
        elif v<=1 and start is not None:
            runs.append(i-start); start=None
    if start is not None: runs.append(len(dark)-start)
    # single text line runs are the short ones; take the lower quartile-ish representative
    runs = sorted(r for r in runs if r>=4)
    if not runs: continue
    med = statistics.median(runs[:max(3,int(len(runs)*0.6))])
    cm_per_px = ch/h
    pt = med*cm_per_px*28.3465
    print(f"{name}: glyph≈{med:.0f}px of {h}px tall; displayed {ch}cm -> ≈{pt:.1f}pt")
