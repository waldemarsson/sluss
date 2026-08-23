# Optional Fish convenience: `sluss-cd NAME` enters a managed worktree.
function sluss-cd --description 'Change directory to a sluss worktree'
    if test (count $argv) -ne 1
        command sluss help
        return 2
    end
    cd (command sluss path "$argv[1]")
end
