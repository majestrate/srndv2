from PIL import Image

def render(infname, outfname, thumbsize=300):
    """
    render a thumbanil for a file
    """
    im = Image.open(infname)
    w, h = im.size
    aspect = w / h
    if aspect > 0:
        nw = aspect * thumbsize
        nh = aspect * thumbsize
        if nw < w and nh < h:
            im.thumbnail((nw,nh))
    im.save(outfname)
    
