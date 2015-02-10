from . import util


def test_article_valid():
    assert util.is_valid_article_id('<ayyy@lmao>')
    assert util.is_valid_article_id('<admin@lel.tld>')
    assert util.is_valid_article_id('<hue.lol@ben.is>')
    assert util.is_valid_article_id('<oajxgwzice1423599709@web.overchan.lolz>')
    assert util.is_valid_article_id(b'<oajxgwzice1423599709@web.overchan.lolz>')

def test_article_invalid():
    assert not util.is_valid_article_id('<admin@lel.tld')
    assert not util.is_valid_article_id('admin@lel.tld')
    assert not util.is_valid_article_id('admin@lel.tld>')
    assert not util.is_valid_article_id('<>admin@lel.tld')
    assert not util.is_valid_article_id('>admin@lel.tld')
    assert not util.is_valid_article_id('>admin@lel.tld<')
    assert not util.is_valid_article_id(':DDDD-benis')
    assert not util.is_valid_article_id('<@lol.tld>')
