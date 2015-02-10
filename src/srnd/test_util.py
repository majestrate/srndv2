from . import util


def test_article_valid():
    assert util.is_valid_article_id('<ayyy@lmao>')
    assert util.is_valid_article_id('<admin@lel.tld>')
    assert util.is_valid_article_id('<hue.lol@ben.is>')


def test_article_invalid():
    assert not util.is_valid_article_id('<admin@lel.tld')
    assert not util.is_valid_article_id('admin@lel.tld')
    assert not util.is_valid_article_id('admin@lel.tld>')
    assert not util.is_valid_article_id('<>admin@lel.tld')
    assert not util.is_valid_article_id('>admin@lel.tld')
    assert not util.is_valid_article_id('>admin@lel.tld<')
    assert not util.is_valid_article_id(':DDDD-benis')
    assert not util.is_valid_article_id('<@lol.tld>')
