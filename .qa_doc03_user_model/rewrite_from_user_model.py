# -*- coding: utf-8 -*-
from pathlib import Path
from copy import deepcopy
import zipfile, shutil, os
from lxml import etree
from PIL import Image, ImageDraw, ImageFont
import math

BASE = Path(r'D:\VST DEVELOP\aviation-journey-agent')
SRC = BASE / 'docs' / '03_软件需求规格说明书.docx'
OUT = BASE / 'docs' / '03_软件需求规格说明书_用例模型修订.docx'
WORK = BASE / '.qa_doc03_user_model'
DIAGRAM = WORK / 'revised_image2.png'
WORK.mkdir(parents=True, exist_ok=True)
FONT = r'C:\Windows\Fonts\msyh.ttc'
FONT_B = r'C:\Windows\Fonts\msyhbd.ttc'


def font(size, bold=False):
    return ImageFont.truetype(FONT_B if bold else FONT, size, index=0)


def measure(draw, text, f):
    b = draw.textbbox((0, 0), text, font=f)
    return b[2] - b[0], b[3] - b[1]


def wrap(draw, text, f, maxw):
    lines, cur = [], ''
    for ch in text:
        if cur and measure(draw, cur + ch, f)[0] > maxw:
            lines.append(cur); cur = ch
        else:
            cur += ch
    if cur: lines.append(cur)
    return lines or ['']


def center_text(draw, box, text, f, fill=(25,25,25), spacing=5):
    x1,y1,x2,y2=box
    maxw=max(1,x2-x1-40)
    lines=[]
    for line in str(text).split('\n'):
        lines.extend(wrap(draw,line,f,maxw))
    hs=[measure(draw,l,f)[1] for l in lines]
    y=y1+(y2-y1-(sum(hs)+spacing*(len(lines)-1)))/2
    for h,l in zip(hs,lines):
        w,_=measure(draw,l,f)
        draw.text((x1+(x2-x1-w)/2,y),l,font=f,fill=fill)
        y+=h+spacing


def actor(draw,x,y,label,f):
    color=(40,40,40)
    draw.ellipse((x-22,y-82,x+22,y-38),outline=color,width=5)
    draw.line((x,y-38,x,y+48),fill=color,width=5)
    draw.line((x-58,y-2,x+58,y-2),fill=color,width=5)
    draw.line((x,y+48,x-46,y+102),fill=color,width=5)
    draw.line((x,y+48,x+46,y+102),fill=color,width=5)
    center_text(draw,(x-170,y+108,x+170,y+190),label,f,fill=color)


def usecase(draw,box,text,f):
    draw.ellipse(box,fill=(250,253,255),outline=(70,90,110),width=4)
    center_text(draw,box,text,f,fill=(25,30,40))


def arrow(draw,p1,p2,color=(55,65,75),width=4,head=16,dashed=False,open_head=False):
    x1,y1=p1; x2,y2=p2
    L=math.hypot(x2-x1,y2-y1)
    if L == 0:
        return
    ux=(x2-x1)/L
    uy=(y2-y1)/L
    if dashed:
            pos=0
            while pos < L:
                a=min(pos,L); b=min(pos+13,L)
                draw.line((x1+ux*a,y1+uy*a,x1+ux*b,y1+uy*b),fill=color,width=width)
                pos+=25
    else:
        draw.line((x1,y1,x2,y2),fill=color,width=width)
    ang=math.atan2(y2-y1,x2-x1)
    p3=(x2+head*math.cos(ang+2.65),y2+head*math.sin(ang+2.65))
    p4=(x2+head*math.cos(ang-2.65),y2+head*math.sin(ang-2.65))
    if open_head:
        draw.line((p3[0],p3[1],x2,y2),fill=color,width=width)
        draw.line((p4[0],p4[1],x2,y2),fill=color,width=width)
    else:
        draw.polygon([(x2,y2),p3,p4],fill=color)


def draw_model(path):
    W,H=2600,1850
    im=Image.new('RGB',(W,H),'white')
    d=ImageDraw.Draw(im)
    d.rectangle((20,20,W-20,H-20),outline=(90,100,110),width=4)
    d.text((60,45),'系统用例模型（修订）',font=font(44,True),fill=(20,30,45))
    bx1,by1,bx2,by2=560,180,2050,1540
    d.rounded_rectangle((bx1,by1,bx2,by2),radius=22,outline=(55,70,85),width=6,fill=(252,253,255))
    d.text((bx1+40,by1+35),'基于 Agent 的民航旅客状态感知与行动决策系统',font=font(30,True),fill=(35,50,65))

    cases=[
        ((750,320,1250,485),'获取个人行程信息数据'),
        ((750,580,1250,745),'获取行程安排建议'),
        ((750,840,1250,1005),'获取行程延误风险'),
        ((750,1090,1250,1255),'导航到机场'),
        ((1510,660,1960,825),'数据处理分析'),
    ]
    centers=[((x1+x2)/2,(y1+y2)/2) for (x1,y1,x2,y2), _ in cases]
    for box,label in cases:
        usecase(d,box,label.replace('获取','获取\n') if label.startswith('获取') else label, font(30,True))
    # Actors, all outside system boundary.
    actor(d,330,650,'旅客',font(33,True))
    actor(d,2320,850,'数据来源',font(33,True))

    # Passenger associations.
    for i in range(4):
        cy=centers[i][1]
        arrow(d,(430,710),(750,cy),width=4,head=16)
    # Data source association.
    arrow(d,(2050,850),(1960,742),width=4,head=16)

    # extend arrows: data processing -> each user-facing use case.
    ext_color=(80,95,110)
    for i in range(4):
        sy,sx=centers[4]
        ey=centers[i][1]
        arrow(d,(1510,742),(1250,ey),color=ext_color,width=4,head=16,dashed=True)
        d.text((1285,ey-46),'<<extend>>',font=font(22,False),fill=ext_color)

    d.text((600,1545),'数据处理分析通过 <<extend>> 扩展四个由旅客直接使用的用例。',font=font(25,False),fill=(60,65,70))
    d.text((60,1715),'实线：参与者与用例的关联；虚线箭头：<<extend>>。',font=font(27,False),fill=(55,65,75))
    im.save(path)


draw_model(DIAGRAM)

W_NS='http://schemas.openxmlformats.org/wordprocessingml/2006/main'
NS={'w':W_NS}
XML='http://www.w3.org/XML/1998/namespace'
with zipfile.ZipFile(SRC,'r') as zin:
    entries={n:zin.read(n) for n in zin.namelist()}
entries['word/media/image2.png']=DIAGRAM.read_bytes()
root=etree.fromstring(entries['word/document.xml'],parser=etree.XMLParser(remove_blank_text=False))
body=root.find('w:body',NS)
if body is None: raise RuntimeError('body not found')

def text_of(p):
    return ''.join(t.text or '' for t in p.xpath('.//w:t',namespaces=NS))

def find_para(exact):
    for p in body.xpath('./w:p',namespaces=NS):
        if text_of(p)==exact:
            return p
    return None

def remove_range(start_text,end_text):
    start=find_para(start_text); end=find_para(end_text)
    if start is None or end is None:
        missing=[]
        if start is None: missing.append(start_text[:40])
        if end is None: missing.append(end_text[:40])
        raise RuntimeError('range not found: '+repr(missing))
    ps=body.xpath('./w:p',namespaces=NS)
    i=ps.index(start); j=ps.index(end)
    for p in ps[i:j+1]:
        body.remove(p)

def replace_text(p,new_text):
    pPr=p.find('w:pPr',NS)
    rPr=None
    for r in p.findall('w:r',NS):
        rp=r.find('w:rPr',NS)
        if rp is not None:
            rPr=deepcopy(rp); break
    for child in list(p):
        if child is pPr: continue
        p.remove(child)
    r=etree.SubElement(p,f'{{{W_NS}}}r')
    if rPr is not None: r.append(rPr)
    t=etree.SubElement(r,f'{{{W_NS}}}t')
    t.set(f'{{{XML}}}space','preserve')
    t.text=new_text

def clone_para_with_text(template, text):
    p=deepcopy(template)
    pPr=p.find('w:pPr',NS)
    rPr=None
    for r in p.findall('w:r',NS):
        rp=r.find('w:rPr',NS)
        if rp is not None:
            rPr=deepcopy(rp); break
    for ch in list(p):
        if ch is pPr: continue
        p.remove(ch)
    r=etree.SubElement(p,f'{{{W_NS}}}r')
    if rPr is not None: r.append(rPr)
    t=etree.SubElement(r,f'{{{W_NS}}}t')
    t.set(f'{{{XML}}}space','preserve')
    t.text=text
    return p

# 1. Delete all sequence-diagram blocks first.
for a,b in [
 ('2.2.1.2 创建行程用例顺序图','图2-2 系统“创建行程并生成初始建议”用例的顺序图'),
 ('2.2.2.2 更新位置用例顺序图','图2-3 系统“更新旅客位置并重新分析”用例的顺序图'),
 ('2.2.3.2 定时刷新用例顺序图','图2-4 系统“定时刷新与实时推送”用例的顺序图'),
 ('2.2.4.2 查看行动建议用例顺序图','图2-5 系统“查看行动建议并跳转导航”用例的顺序图'),
]:
    remove_range(a,b)

# 2. Section 2.1 + 2.2 intro.
replace_text(find_para('系统的外部参与者包括旅客和数据来源。旅客可以获取个人行程信息，获取行程安排建议，获取行程延误风险，导航到机场；系统通过数据感知 Agent 从航班、机场与路线数据源获取数据，支撑行程状态与行动建议的生成。系统用例模型如图2-1所示。'),
 '系统的外部参与者包括旅客和数据来源。旅客直接使用“获取个人行程信息数据”“获取行程安排建议”“获取行程延误风险”和“导航到机场”四个用例；数据来源参与“数据处理分析”用例，为系统提供航班、机场、路线等最新数据。数据处理分析通过 <<extend>> 扩展旅客直接使用的四个用例，行程信息、建议、风险和导航均以经处理的数据为依据。系统用例模型如图2-1所示。')
replace_text(find_para('2.2 软件需求的用例描述及分析顺序图'),'2.2 软件需求的用例描述')
replace_text(find_para('系统的主要用例包括创建行程并生成初始建议、更新旅客位置并重新分析、定时刷新与实时推送、查看行动建议并跳转导航。每个用例均包含用例描述与对应顺序图，顺序图中使用边界类、控制类和实体类标注参与交互的各类对象。'),
 '系统的主要用例包括旅客直接使用的“获取个人行程信息数据”“获取行程安排建议”“获取行程延误风险”“导航到机场”，以及由数据来源参与的“数据处理分析”。数据处理分析是扩展用例，为前四个用例提供数据支撑。本节按上述五个用例分别描述业务目标、执行者、触发条件、前置条件、基本交互动作、扩展交互动作及非功能要求；对应顺序图暂以占位符标记，待后续设计阶段统一补绘。')

# 3. Use case descriptions.
replace_text(find_para('2.2.1 用例名：行程初始化及建议'),'2.2.1 用例名：获取个人行程信息数据')
replace_text(find_para('2.2.2 用例名：更新旅客位置并重新分析'),'2.2.2 用例名：获取行程安排建议')
replace_text(find_para('2.2.3 用例名：定时刷新与实时推送'),'2.2.3 用例名：获取行程延误风险')
replace_text(find_para('2.2.4 用例名：查看行动建议并跳转导航'),'2.2.4 用例名：导航到机场')

# 3. Use case descriptions.
def replace_description(subheading_old, subheading_new, body_end, labels):
    h=find_para(subheading_old)
    if h is None: raise RuntimeError('heading not found: '+subheading_old)
    replace_text(h,subheading_new)
    end=find_para(body_end)
    if end is None: raise RuntimeError('body end not found: '+body_end)
    ps=body.xpath('./w:p',namespaces=NS)
    hpos=ps.index(h); epos=ps.index(end)
    template=ps[hpos+1]
    # remove old body (including heading? no, heading stays)
    for p in ps[hpos+1:epos+1]:
        body.remove(p)
    # insert labels just after heading using the template
    anchor=h
    for text in labels:
        newp=clone_para_with_text(template,text)
        anchor.addnext(newp); anchor=newp

# Case 1
replace_description(
 '2.2.1.1 创建行程用例描述','2.2.1.1 获取个人行程信息数据用例描述',
 '可靠性需求：数据缺失、接口异常或输入不完整时给出明确提示而不崩溃，并展示数据更新时间与来源。',
 [
  '业务目标：旅客获取当前航班、机场、路线和旅客自身位置等个人行程信息，并了解数据的来源与更新时间。',
  '执行者：旅客（发起者）；数据来源（通过“数据处理分析”参与）。',
  '触发条件：旅客进入行程页面，或主动刷新个人行程信息。',
  '前置条件：系统服务可用；已存在可查询的行程；数据来源可访问或已有最近一次有效数据。',
  '基本交互动作：',
  '1. 旅客请求查看个人行程信息。',
  '2. 系统识别当前行程及其涉及的航班、机场、路线和旅客位置。',
  '3. 数据处理分析获取并标准化相关数据，记录数据来源与更新时间。',
  '4. 系统返回并展示个人行程信息、缺失或降级状态。',
  '扩展交互动作：',
  '1a. 数据来源不可用：使用最近一次有效数据并明确标注数据时间和降级状态。',
  '2a. 定位不可用：允许旅客查看已有行程信息，并提示当前定位信息不可用。',
  '后置条件：旅客获取到带数据来源、更新时间和有效性的个人行程信息。',
  '业务规则：系统只展示有依据的数据，不虚构航班、机场、路线或位置状态。',
  '性能需求：请求应在可接受时间内返回；外部数据查询应设置超时。',
  '可靠性需求：单个数据源失败不影响其他行程信息展示。',
  '【顺序图占位符】本用例对应的 UML 顺序图待后续补充。',
 ])

# Case 2
replace_description(
 '2.2.2.1 更新位置用例描述','2.2.2.1 获取行程安排建议用例描述',
 '可靠性需求：定位失效时系统给出降级提示，不因定位异常而中断服务。',
 [
  '业务目标：旅客获取由系统生成的、基于当前行程状态和个人行程信息的下一步行动安排建议。',
  '执行者：旅客（发起者）；数据来源（通过“数据处理分析”参与）。',
  '触发条件：旅客查看行程安排建议，或行程状态发生变化后系统重新生成建议。',
  '前置条件：已获取个人行程信息；数据来源可用或已有最近一次有效数据。',
  '基本交互动作：',
  '1. 旅客请求获取行程安排建议。',
  '2. 系统读取当前行程信息与数据处理分析结果。',
  '3. 系统根据当前时间、旅客所处阶段和剩余时间生成有优先级的行动建议。',
  '4. 客户端展示建议、风险等级、建议依据和生成时间。',
  '扩展交互动作：',
  '1a. 行程数据不完整或过期：使用最近一次有效数据并给出降级提示。',
  '2a. 当前定位不可用：基于最近一次有效位置生成建议，并提示位置不确定性。',
  '3a. 系统无法确定下一步：明确提示旅客向航司或机场工作人员确认，不输出确定性结论。',
  '后置条件：旅客获得可解释、可执行的行程安排建议。',
  '业务规则：值机、登机等关键时间作为硬约束；系统不自动执行改签、支付等高影响操作。',
  '性能需求：建议生成应在可接受时间内完成，并支持数据变化后的及时更新。',
  '可靠性需求：数据缺失或接口异常时仍可展示基础提示，不因单点故障中断服务。',
  '【顺序图占位符】本用例对应的 UML 顺序图待后续补充。',
 ])

# Case 3
replace_description(
 '2.2.3.1 定时刷新用例描述','2.2.3.1 获取行程延误风险用例描述',
 '可靠性需求：系统长时间运行不因个别数据源异常而终止，支持断线重连与状态恢复。',
 [
  '业务目标：旅客获取航班延误、机场运行、交通和路径变化等因素导致的行程延误风险。',
  '执行者：旅客（发起者）；数据来源（通过“数据处理分析”参与）。',
  '触发条件：旅客查看行程风险，或航班、机场、路线等状态发生变化。',
  '前置条件：已获取个人行程信息；数据来源可用或已有最近一次有效数据。',
  '基本交互动作：',
  '1. 旅客请求获取行程延误风险。',
  '2. 系统读取当前航班、机场、路线、时间和旅客位置状态。',
  '3. 数据处理分析对最新数据进行标准化、校验和降级判断。',
  '4. 系统分析风险等级、主要风险因素和可能影响。',
  '5. 客户端展示风险等级、风险说明、数据来源和更新时间。',
  '扩展交互动作：',
  '1a. 数据来源不可用：使用最近一次有效数据并明确提示风险判断时效有限。',
  '2a. 航班或机场状态未知：标记为未知，不输出确定性延误结论。',
  '后置条件：旅客获取到有时间窗口和数据依据的行程延误风险。',
  '业务规则：风险展示以航班、机场和交通的最新可用数据为依据；无法确认时不夸大风险。',
  '性能需求：风险数据应按合理周期更新，数据变化后应及时重新分析。',
  '可靠性需求：外部数据异常或延迟时，系统应继续运行并显示最后更新时间。',
  '【顺序图占位符】本用例对应的 UML 顺序图待后续补充。',
 ])

# Case 4
replace_description(
 '2.2.4.1 查看行动建议用例描述','2.2.4.1 导航到机场用例描述',
 '可靠性需求：导航跳转失败时提示旅客手动打开导航应用，不影响行程状态的展示。',
 [
  '业务目标：旅客通过系统一键导航到机场，并在地图应用或浏览器不可用时获得清晰的替代提示。',
  '执行者：旅客（发起者）；系统根据当前定位生成导航地址。',
  '触发条件：旅客查看行程信息或行动建议后，点击“导航到机场”。',
  '前置条件：已获取当前位置或最近一次有效位置；已确定机场目的地。',
  '基本交互动作：',
  '1. 旅客点击“导航到机场”。',
  '2. 系统读取当前位置、机场目的地和可用导航服务。',
  '3. 系统生成包含当前位置与机场目的地的导航地址。',
  '4. 系统跳转到地图导航应用；应用未安装时回退为浏览器打开导航网页。',
  '5. 返回系统后，客户端继续展示机场内值机、安检、登机口等文字指引。',
  '扩展交互动作：',
  '1a. 当前位置不可用：使用手动选择的位置或最近一次有效位置生成导航，并提示定位精度。',
  '2a. 导航应用未安装：回退为使用浏览器打开导航网页。',
  '3a. 导航跳转失败：提示旅客手动打开导航应用，不影响其他行程信息展示。',
  '后置条件：导航已启动或已提示旅客可用替代方式。',
  '业务规则：系统只负责跳转导航，不获取或保存导航过程中的轨迹；高影响操作仍需旅客确认。',
  '性能需求：点击导航后应尽快响应并完成跳转。',
  '可靠性需求：导航失败时应有明确提示，并保持行程状态持续更新。',
  '【顺序图占位符】本用例对应的 UML 顺序图待后续补充。',
 ])

# Case 5 inserted after case 4 placeholder.
def insert_after(anchor, texts):
    for text in texts:
        # template: use the placeholder paragraph itself, preserving normal style
        newp=clone_para_with_text(anchor,text)
        anchor.addnext(newp); anchor=newp

placeholders=body.xpath('./w:p',namespaces=NS)
case4_ph=next(p for p in placeholders if text_of(p)=='【顺序图占位符】本用例对应的 UML 顺序图待后续补充。')
# There are four identical placeholders; choose the last one (case 4) by locating heading before.
# Better find the placeholder after 导航到机场 heading: iterate body paragraphs.
ph=None
for p in body.xpath('./w:p',namespaces=NS):
    txt=text_of(p)
    if txt=='2.2.4 用例名：导航到机场':
        ph=None
    elif txt=='【顺序图占位符】本用例对应的 UML 顺序图待后续补充。':
        ph=p
if ph is None: raise RuntimeError('case4 placeholder not found')
insert_after(ph,[
 '2.2.5 用例名：数据处理分析',
 '2.2.5.1 数据处理分析用例描述',
 '业务目标：数据来源向系统提供航班、机场、路线等原始数据，系统完成获取、标准化、校验和状态更新，为旅客直接使用的四个用例提供数据支撑。',
 '执行者：数据来源（发起者）。',
 '触发条件：旅客触发任一数据相关用例，或系统需要更新行程数据时。',
 '前置条件：系统服务可用；数据来源可访问或已有最近一次有效数据。',
 '基本交互动作：',
 '1. 系统向数据来源请求航班、机场、路线等数据。',
 '2. 数据来源返回原始数据或错误状态。',
 '3. 系统对数据进行分析、标准化并识别缺失、过期或异常项。',
 '4. 系统记录数据来源、更新时间和数据质量状态。',
 '5. 处理后的数据供“获取个人行程信息数据”“获取行程安排建议”“获取行程延误风险”和“导航到机场”使用。',
 '扩展交互动作：',
 '1a. 数据来源不可用：系统使用最近一次有效数据并明确标注降级状态。',
 '2a. 数据格式或内容异常：系统记录问题并向旅客给出明确提示，不编造数据。',
 '后置条件：系统获得可使用或可降级的最新数据，并保留数据来源与更新时间。',
 '业务规则：数据处理分析是系统内部扩展用例，不自动执行改签、支付等高影响操作。',
 '性能需求：数据查询应设置超时；单个数据来源失败不应阻塞其他数据处理。',
 '可靠性需求：系统在数据缺失、延迟或异常时继续运行，并展示明确的质量提示。',
 '【顺序图占位符】本用例对应的 UML 顺序图待后续补充。',
])

# Final paragraph referencing sequence diagrams.
fp=find_para('图中各类与用例顺序图的对应关系为：旅客客户端界面对应顺序图中的边界类；行程服务、数据感知 Agent、行动决策 Agent、实时推送对应顺序图中的控制类；行程状态、行动建议对应顺序图中的实体类。顺序图中出现的消息与参数均可在对应类的方法或属性中体现。')
if fp is not None:
    replace_text(fp,'图中各类与前述软件需求中的系统组件、数据对象和处理过程相对应。')

# Remove relationships and package parts for deleted sequence diagrams.
remove_media={'word/media/image3.png','word/media/image4.png','word/media/image5.png','word/media/image6.png'}
rels_root=etree.fromstring(entries['word/_rels/document.xml.rels'])
rel_ns='http://schemas.openxmlformats.org/package/2006/relationships'
for rel in list(rels_root):
    target=rel.get('Target') or ''
    if any(target == m.split('/')[-1] or target.endswith('/'+m.split('/')[-1]) for m in remove_media):
        rels_root.remove(rel)
entries['word/_rels/document.xml.rels']=etree.tostring(rels_root,xml_declaration=True,encoding='UTF-8',standalone=True)
entries.pop('word/media/image3.png',None)
entries.pop('word/media/image4.png',None)
entries.pop('word/media/image5.png',None)
entries.pop('word/media/image6.png',None)

entries['word/document.xml']=etree.tostring(root,xml_declaration=True,encoding='UTF-8',standalone=True)
with zipfile.ZipFile(OUT,'w',compression=zipfile.ZIP_DEFLATED) as zout:
    for name,data in entries.items():
        zout.writestr(name,data)
print('WROTE',OUT)
print('DIAGRAM',DIAGRAM)
