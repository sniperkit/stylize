from stylize.formatter import Formatter
from stylize.util import *

import os
import subprocess
import shutil
import tempfile


class ClangFormatter(Formatter):
    def __init__(self):
        super().__init__()
        self.clang_command = self.get_command()
        self._config_file_name = ".clang-format"
        self.file_extensions = [".c", ".h", ".cpp", ".hpp"]
        self._tempdir = tempfile.mkdtemp()

    def add_args(self, argparser):
        argparser.add_argument(
            "--clang_style",
            type=str,
            default=None,
            help=
            "The style to pass to clang-format.  See `clang-format --help` for more info.")

    def run(self, args, filepath, check=False, calc_diff=False):
        logfile = open("/dev/null", "w")
        if check or calc_diff:
            popen_args = [self.clang_command]
            if args.clang_style:
                popen_args.append("-style=%s" % args.clang_style)
            popen_args.append(filepath)
            outfile_path = os.path.join(self._tempdir, filepath)

            # write style-compliant version of file to a tmp directory
            outfile = open(outfile_path, 'w')
            proc = subprocess.Popen(popen_args, stdout=outfile)
            proc.communicate()
            outfile.close()

            # TODO: Popen exit codes?

            # note: filepath[2:] cuts off leading './'
            patch = calculate_diff(filepath, outfile_path, filepath[2:])
            noncompliant = len(patch) > 0

            return noncompliant, patch
        else:
            md5_before = file_md5(filepath)
            popen_args = [self.clang_command, "-i"]
            if args.clang_style:
                popen_args.append("-style=%s" % args.clang_style)
            popen_args.append(filepath)
            proc = subprocess.Popen(popen_args, stdout=logfile, stderr=logfile)
            proc.communicate()
            md5_after = file_md5(filepath)
            return (md5_before != md5_after), None

    def get_command(self):
        if shutil.which("clang-format") != None:
            return "clang-format"
        # Run the next command in bash, as we need the bash builtin, compgen
        possible_command = subprocess.Popen(
            ["bash", "-c",
             "compgen -A function -abck | grep -E clang-format-[0-9]+\.[0-9]+ | tail -n 1"
             ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE).stdout.read().decode("utf-8").strip()
        if possible_command != "":
            return possible_command
        else:
            return None
