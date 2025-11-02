from flask import Flask, request, jsonify
from multilspy import SyncLanguageServer
from multilspy.multilspy_config import MultilspyConfig
from multilspy.multilspy_logger import MultilspyLogger
from multilspy.multilspy_types import Location
from multilspy.multilspy_types import Position
from pprint import pprint
from dataclasses import dataclass
import sys


app = Flask(__name__)
lsp : SyncLanguageServer

@app.route('/hello', methods=['GET'])
def hello():
    return jsonify(code = 0, res = "hello world!")

@app.route('/setup', methods=['POST'])
def setup():
    data = request.get_json()
    pprint(data)
    lang = data["lang"]
    root = data["root"]
    global lsp
    config = MultilspyConfig.from_dict({"code_language": lang})
    logger = MultilspyLogger()
    lsp = SyncLanguageServer.create(config, logger, root)
    return jsonify(code = 0)

@app.route('/definition', methods=['GET'])
def requestDefinition():
    global lsp
    data = request.get_json()
    if not data :
        return jsonify(code = 1)
    pprint(data)
    args = RequestArgs(**data)
    res = []
    with lsp.start_server():
        lsp.open_file(args.file)
        for loc in args.loc:
            def_loc = lsp.request_definition(args.file, loc['line'], loc['column'])
            if len(def_loc) != 0:
                res.append(def_loc)

    return jsonify(code = 0, res = res)

@app.route('/reference', methods=['GET'])
def requestReference():
    global lsp
    data = request.get_json()
    if not data :
        return jsonify(code = 1)
    pprint(data)
    args = RequestArgs(**data)
    res = []
    with lsp.start_server():
        lsp.open_file(args.file)
        for loc in args.loc:
            allRef = lsp.request_references(args.file, loc['line'], loc['column'])
            allRef_simple = []
            for ref in allRef:
                allRef_simple.append(Loc(ref=ref))
            res.append(allRef_simple)

    return jsonify(code = 0, res = res)

@dataclass
class Loc:
    relFile : str
    pos : list[int]

    def __init__(self, ref: Location):
        self.relFile = ref['relativePath']
        pos = ref['range']["start"]
        self.pos = [pos["line"], pos['character']]


@dataclass
class RequestArgs:
    file :str
    loc : list[dict]

if __name__ == '__main__':
    port = sys.argv[1]
    app.run(host='0.0.0.0', port=port)
    # config = MultilspyConfig.from_dict({"code_language": "go"})
    # logger = MultilspyLogger()
    # lsp = SyncLanguageServer.create(config, logger, "/root/workspace/llm_dev")
    # with lsp.start_server():
    #     print("hello")
    #     lsp.open_file("agent/baseAgent.go")
    #     res, tree= lsp.request_document_symbols("agent/baseAgent.go")
    #     pprint(res)
    #     loc: Location = res[0]["location"]
    #     ref = lsp.request_references("agent/baseAgent.go", 15, 5)
    #     pprint(ref)
