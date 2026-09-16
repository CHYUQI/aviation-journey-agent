# -*- coding: utf-8 -*-
import sys, os, re, zipfile, shutil
from docx import Document
from docx.oxml.ns import qn

def extract(src, outdir, tag):
    os.makedirs(outdir, exist_ok=True)
    imgdir = os.path.join(outdir, 'images')
    os.makedirs(imgdir, exist_ok=True)
    doc = Document(src)
    part = doc.part
    # map rId -> image part
    rid_map = {}
    for rid, rel in part.rels.items():
        if 'image' in rel.reltype:
            rid_map[rid] = rel.target_part
    lines = []
    img_index = 0
    # iterate body children in order
    body = doc.element.body
    from docx.table import Table
    from docx.text.paragraph import Paragraph
    def handle_paragraph(p, where=''):
        nonlocal img_index
        par = Paragraph(p, doc)
        txt = par.text.strip()
        style = par.style.name if par.style is not None else ''
        # find drawings / blips
        blips = p.findall('.//' + qn('a:blip'))
        rids = []
        for b in blips:
            rid = b.get(qn('r:embed'))
            if rid and rid not in rids:
                rids.append(rid)
        if txt:
            prefix = ''
            if style.startswith('Heading') or style.startswith('标题'):
                prefix = '### [%s] ' % style
            lines.append(prefix + txt)
        for rid in rids:
            img_index += 1
            tp = rid_map.get(rid)
            if tp is None:
                lines.append('[[IMAGE %03d rid=%s UNRESOLVED]]' % (img_index, rid))
                continue
            ext = os.path.splitext(tp.partname)[1] or '.png'
            fn = '%s_fig%03d%s' % (tag, img_index, ext)
            path = os.path.join(imgdir, fn)
            with open(path, 'wb') as f:
                f.write(tp.blob)
            lines.append('[[IMAGE %03d file=%s size=%d bytes]]' % (img_index, fn, len(tp.blob)))
    def handle_table(tbl):
        lines.append('[TABLE]')
        for row in tbl.rows:
            cells = []
            for c in row.cells:
                cells.append(' '.join(c.text.split()))
            lines.append(' | '.join(cells))
        lines.append('[/TABLE]')
    for child in body.iterchildren():
        if child.tag == qn('w:p'):
            handle_paragraph(child)
        elif child.tag == qn('w:tbl'):
            handle_table(Table(child, doc))
        elif child.tag == qn('w:sectPr'):
            pass
    with open(os.path.join(outdir, '%s_text.txt' % tag), 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))
    print(tag, 'paras/images:', img_index)

extract(sys.argv[1], sys.argv[2], sys.argv[3])
