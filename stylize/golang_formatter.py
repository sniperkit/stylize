from stylize.formatter import Formatter
from stylize.util import *

import os
import shutil
import subprocess
import tempfile


class GolangFormatter(Formatter):
    def __init__(self):
        super().__init__()
        self.file_extensions = [".go"]
        self._tempdir = tempfile.mkdtemp()

    def run(self, args, filepath, check=False, calc_diff=False):
        logfile = open("/dev/null", "w")
        if check or calc_diff:
            # write style-compliant version of file to a tmp directory
            outfile_path = os.path.join(self._tempdir, filepath)
            os.makedirs(os.path.dirname(outfile_path), exist_ok=True)
            outfile = open(outfile_path, 'w')
            proc = subprocess.Popen(
                ["gofmt", filepath], stdout=outfile, stderr=subprocess.PIPE)
            out, err = proc.communicate()
            outfile.close()

            # return code zero indicates style-compliant file. 2 indicates non-
            # compliance.  Other return codes indicate errors.
            # TODO: update this comment and check - these were leftover from copying the yapf file
            if proc.returncode != 0 and proc.returncode != 2:
                raise RuntimeError("Call to gofmt failed for file '%s':\n%s" %
                                   (filepath, err.decode('utf-8')))

            # note: filepath[2:] cuts off leading './'
            patch = calculate_diff(filepath, outfile_path, filepath)
            noncompliant = len(patch) > 0

            return noncompliant, patch
        else:
            md5_before = file_md5(filepath)
            proc = subprocess.Popen(
                ["gofmt", "-l", "-w", filepath], stdout=logfile, stderr=logfile)
            proc.communicate()
            md5_after = file_md5(filepath)
            return (md5_before != md5_after), None

    def get_command(self):
        return shutil.which("gofmt")
