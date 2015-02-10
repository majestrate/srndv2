#
# test_nntp.py
#

from . import nntp

def test_nntp_policy_rule_inverted():
    rule = nntp.PolicyRule('!overchan.lame')
    assert rule.allows_newsgroup('overchan.awesome')
    assert not rule.allows_newsgroup('overchan.lame')
    assert rule.allows_newsgroup('overchan.lamecat')
    assert rule.allows_newsgroup('alt.bin.hax')

def test_nntp_policy_rule_regular():
    rule = nntp.PolicyRule('overchan.lame')
    assert not rule.allows_newsgroup('overchan.awesome')
    assert rule.allows_newsgroup('overchan.lame')
    assert not rule.allows_newsgroup('overchan.lamecat')
    assert not rule.allows_newsgroup('alt.bin.hax')
    
def test_nntp_policy_rule_regex():
    rule = nntp.PolicyRule('overchan.*')
    assert rule.allows_newsgroup('overchan.awesome')
    assert rule.allows_newsgroup('overchan.lame')
    assert rule.allows_newsgroup('overchan.lamecat')
    assert not rule.allows_newsgroup('alt.bin.hax')

def test_nntp_policy_rule_inverted_regex():
    rule = nntp.PolicyRule('!overchan.*')
    assert not rule.allows_newsgroup('overchan.awesome')
    assert not rule.allows_newsgroup('overchan.lame')
    assert not rule.allows_newsgroup('overchan.lamecat')
    assert rule.allows_newsgroup('alt.bin.hax')
    
    
